import { afterEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { type JsonSchema, toZod, type Zod } from "../extensions/omp/agent-message-bus/lib/schema.ts";
import { createBusExtension } from "../extensions/omp/agent-message-bus/index.ts";
import { BusError, type BusClient, createClient, type FetchLike, type RemoteTool } from "../extensions/omp/agent-message-bus/lib/client.ts";
import { runCommand, type CommandDeps, USAGE } from "../extensions/omp/agent-message-bus/lib/command.ts";
import { conflictWarning } from "../extensions/omp/agent-message-bus/lib/conflict.ts";
import {
	type CredentialStore,
	configDir,
	createCredentialStore,
	resolveActive,
	type SessionCredentials,
	type StoredCredentials,
} from "../extensions/omp/agent-message-bus/lib/credentials.ts";
import { createDeliver, deliveryOptions } from "../extensions/omp/agent-message-bus/lib/deliver.ts";
import { formatEntry } from "../extensions/omp/agent-message-bus/lib/format.ts";
import { type Entry, runInboxLoop } from "../extensions/omp/agent-message-bus/lib/inbox.ts";
import { parseSse } from "../extensions/omp/agent-message-bus/lib/sse.ts";
import { activateBusTools, deactivateBusTools } from "../extensions/omp/agent-message-bus/lib/activation.ts";
import { resolveBinary } from "../extensions/omp/agent-message-bus/lib/binary.ts";
import { assetName, expectedSum, RELEASES_URL } from "../extensions/omp/agent-message-bus/lib/download.ts";
import { consumeEnv } from "../extensions/omp/agent-message-bus/lib/env.ts";
import type { Runner } from "../extensions/omp/agent-message-bus/lib/runner.ts";
import { createServerCli, type ServerCli } from "../extensions/omp/agent-message-bus/lib/server.ts";
import { INBOX_TOOLS, registerRemoteTools, type RemoteToolsOptions } from "../extensions/omp/agent-message-bus/lib/tools.ts";

/**
 * Recording stand-in for `pi.zod` (no zod package in this repo): nodes keep enough state to rebuild JSON schema.
 * Mirrors omp's omptype-backed shim surface: no `z.strictObject` (regression), objects use `.strict()`.
 */
interface FakeNode {
	type: string;
	description?: string;
	isOptional?: boolean;
	shape?: Record<string, FakeNode>;
	isStrict?: boolean;
	int(): FakeNode;
	optional(): FakeNode;
	describe(text: string): FakeNode;
	strict(): FakeNode;
}
function fakeNode(fields: Partial<FakeNode> & { type: string }): FakeNode {
	const node: FakeNode = {
		...fields,
		int: () => fakeNode({ ...fields, type: "integer" }),
		optional: () => fakeNode({ ...fields, isOptional: true }),
		describe: description => fakeNode({ ...fields, description }),
		strict: () => fakeNode({ ...fields, isStrict: true }),
	} as FakeNode;
	return node;
}
const fakeZ = {
	string: () => fakeNode({ type: "string" }),
	number: () => fakeNode({ type: "number" }),
	boolean: () => fakeNode({ type: "boolean" }),
	object: (shape: Record<string, FakeNode>) => fakeNode({ type: "object", shape }),
};
const z = fakeZ as unknown as Zod;

function toJson(node: FakeNode): JsonSchema {
	const out: JsonSchema = { type: node.type };
	if (node.description) out.description = node.description;
	if (node.shape) {
		out.properties = Object.fromEntries(Object.entries(node.shape).map(([k, v]) => [k, toJson(v)]));
		out.required = Object.entries(node.shape).filter(([, v]) => v.isOptional !== true).map(([k]) => k);
		if (node.isStrict) out.additionalProperties = false;
	}
	return out;
}

const URL_ = "http://bus.test";

function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
	const encoder = new TextEncoder();
	return new ReadableStream({
		start(controller) {
			for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
			controller.close();
		},
	});
}

async function collect<T>(it: AsyncIterable<T>): Promise<T[]> {
	const out: T[] = [];
	for await (const item of it) out.push(item);
	return out;
}

function entry(id: number, extra: Partial<Entry> = {}): Entry {
	return { id, space: "demo", type: "direct_message", from: "lead", critical: false, body: `body ${id}`, createdAt: "t", ...extra };
}

function frame(e: Entry): string {
	return `id: ${e.id}\nevent: inbox\ndata: ${JSON.stringify(e)}\n\n`;
}

function errorResponse(status: number, code: string, message: string): Response {
	return Response.json({ error: { code, message } }, { status });
}

interface Call {
	url: string;
	method: string;
	auth: string | undefined;
	body: unknown;
}

/** Fake bus: each stream open shifts the next scripted response; `/v1/tools` lists `list`; tool calls go to `tools`. */
function fakeBus(
	streams: (() => Response)[],
	tools: Record<string, (args: Record<string, unknown>, auth: string | undefined) => Response> = {},
	list: RemoteTool[] = [],
) {
	const calls: Call[] = [];
	const fetchFn: FetchLike = async (url, init) => {
		calls.push({ url, method: init.method, auth: init.headers.Authorization, body: init.body ? JSON.parse(init.body) : undefined });
		if (url.endsWith("/v1/inbox/stream")) {
			const next = streams.shift();
			return next ? next() : errorResponse(401, "unauthenticated", "invalid token");
		}
		if (url.endsWith("/v1/tools")) return Response.json(list);
		const name = url.slice(url.lastIndexOf("/") + 1);
		const handler = tools[name];
		if (!handler) return Response.json({ result: { acked: 1 } });
		return handler(init.body ? JSON.parse(init.body) : {}, init.headers.Authorization);
	};
	return { calls, fetchFn };
}

const noSleep = async (): Promise<void> => {};

describe("toZod", () => {
	const schema: JsonSchema = {
		type: "object",
		description: "args",
		properties: {
			space: { type: "string", description: "space name" },
			upTo: { type: "integer" },
			critical: { type: "boolean" },
		},
		required: ["space", "upTo"],
		additionalProperties: false,
	};

	test("round-trips the server subset", () => {
		// Regression: omp's shim has no `z.strictObject`; the stand-in lacks it too, so calling it would throw here.
		expect("strictObject" in fakeZ).toBe(false);
		expect(toJson(toZod(schema, z) as unknown as FakeNode)).toEqual(schema);
	});

	test("optional fields keep their description", () => {
		const node = toZod({ type: "object", properties: { a: { type: "string", description: "d" } } }, z) as unknown as FakeNode;
		expect(node.shape?.a).toMatchObject({ type: "string", description: "d", isOptional: true });
		expect(node.isStrict).toBeUndefined();
	});

	test("rejects array", () => {
		expect(() => toZod({ type: "object", properties: { xs: { type: "array" } } }, z)).toThrow("unsupported schema type array");
	});
});

describe("parseSse", () => {
	test("splits frames, joins multi-line data, skips comments, handles CRLF and split chunks", async () => {
		const frames = await collect(
			parseSse(streamOf([": ping\n\n", "id: 1\r\nevent: inbox\r\ndata: a\r\n", "data: b\r\n\r\n", "id: 2\nev", "ent: inbox\ndata: c\n\n", "data: d\n\n"])),
		);
		expect(frames).toEqual([
			{ id: "1", event: "inbox", data: "a\nb" },
			{ id: "2", event: "inbox", data: "c" },
			{ event: "message", data: "d" },
		]);
	});
});

describe("formatEntry", () => {
	test("renders each type", () => {
		expect(formatEntry(entry(1, { critical: true }))).toBe("[agent-bus] demo · direct · from lead · CRITICAL\nbody 1");
		expect(formatEntry(entry(2, { type: "topic_message", topic: "all" }))).toStartWith("[agent-bus] demo · topic #all · from lead\n");
		expect(formatEntry(entry(3, { type: "topic_closed", topic: "x" }))).toContain("· topic #x closed ·");
		expect(formatEntry(entry(4, { type: "thread_post", threadId: 7, threadTitle: "T", blackboard: "bb" }))).toContain('· thread 7 "T" on bb ·');
		expect(formatEntry(entry(5, { type: "thread_removed", threadId: 7, threadTitle: "T" }))).toContain('· thread 7 "T" removed ·');
		expect(formatEntry(entry(6, { type: "blackboard_removed", blackboard: "bb" }))).toContain("· blackboard bb removed ·");
	});
});

describe("deliver", () => {
	test("deliveryOptions: critical steers, others follow up; only tui and rpc trigger a turn", () => {
		for (const [critical, mode, expected] of [
			[true, "tui", { deliverAs: "steer", triggerTurn: true }],
			[false, "rpc", { deliverAs: "followUp", triggerTurn: true }],
			[true, "print", { deliverAs: "steer", triggerTurn: false }],
			[false, "json", { deliverAs: "followUp", triggerTurn: false }],
			[false, undefined, { deliverAs: "followUp", triggerTurn: false }],
		] as const) {
			expect({ critical, mode, options: deliveryOptions(entry(1, { critical }), mode) }).toEqual({ critical, mode, options: expected });
		}
	});

	test("createDeliver sends the formatted entry with the options for the mode at delivery time", async () => {
		const sent: unknown[] = [];
		let mode = "print";
		const pi = { sendMessage: (message: unknown, options: unknown) => void sent.push({ message, options }) } as unknown as ExtensionAPI;
		const deliver = createDeliver(pi, () => mode);
		await deliver(entry(1));
		mode = "tui";
		await deliver(entry(2, { critical: true }));
		expect(sent).toEqual([
			{ message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true }, options: { deliverAs: "followUp", triggerTurn: false } },
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { critical: true })), display: true },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
	});
});

describe("runInboxLoop", () => {
	test("delivers in order, acks each id, reconnects, and stops on 401", async () => {
		const bus = fakeBus([
			() => new Response(streamOf([": ping\n\n", frame(entry(1)), frame(entry(2, { critical: true }))])),
			() => new Response(streamOf([frame(entry(3))])),
		]);
		const delivered: number[] = [];
		const errors: string[] = [];
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: new AbortController().signal,
			deliver: async e => void delivered.push(e.id),
			onError: error => void errors.push(error.message),
			onConflict: () => {},
			sleep: noSleep,
		});
		expect(delivered).toEqual([1, 2, 3]);
		const acks = bus.calls.filter(c => c.url.endsWith("/bus_inbox_ack"));
		expect(acks.map(c => c.body)).toEqual([{ upTo: 1 }, { upTo: 2 }, { upTo: 3 }]);
		expect(acks.every(c => c.auth === "Bearer tok")).toBe(true);
		expect(errors).toEqual(["invalid token"]);
	});

	test("retries other errors with doubling backoff capped at 30s", async () => {
		const bus = fakeBus([
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(500, "internal", "boom"),
			() => errorResponse(403, "forbidden", "admin"),
		]);
		const delays: number[] = [];
		const errors: string[] = [];
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: new AbortController().signal,
			deliver: async () => {},
			onError: error => void errors.push(error.message),
			onConflict: () => {},
			sleep: async ms => void delays.push(ms),
		});
		expect(delays).toEqual([1000, 2000, 4000, 8000, 16000, 30000]);
		expect(errors.at(-1)).toBe("admin");
	});

	test("returns immediately when aborted", async () => {
		const controller = new AbortController();
		controller.abort();
		const bus = fakeBus([]);
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: controller.signal,
			deliver: async () => {},
			onError: () => {},
			onConflict: () => {},
			sleep: noSleep,
		});
		expect(bus.calls).toHaveLength(0);
	});

	test("an ack failure is reported without reconnecting or redelivering", async () => {
		const bus = fakeBus([() => new Response(streamOf([frame(entry(1)), frame(entry(2))]))], {
			bus_inbox_ack: args => (args.upTo === 1 ? errorResponse(500, "internal", "ack failed") : Response.json({ result: { acked: 2 } })),
		});
		const delivered: number[] = [];
		const errors: string[] = [];
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: new AbortController().signal,
			deliver: async e => void delivered.push(e.id),
			onError: error => void errors.push(error.message),
			onConflict: () => {},
			sleep: noSleep,
		});
		expect(delivered).toEqual([1, 2]);
		expect(bus.calls.filter(c => c.url.endsWith("/bus_inbox_ack")).map(c => c.body)).toEqual([{ upTo: 1 }, { upTo: 2 }]);
		expect(errors).toEqual(["ack failed", "invalid token"]);
	});

	test("a stream that opens without frames still ends the conflict streak", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const bus = fakeBus([conflict, () => new Response(streamOf([": ping\n\n"])), conflict]);
		const conflicts: string[] = [];
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: new AbortController().signal,
			deliver: async () => {},
			onError: () => {},
			onConflict: error => void conflicts.push(error.code),
			sleep: noSleep,
		});
		expect(conflicts).toEqual(["conflict", "conflict"]);
	});

	test("a 409 conflict is reported once per streak, retried with backoff, and a successful open resets the streak", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const bus = fakeBus([conflict, conflict, () => new Response(streamOf([frame(entry(1))])), conflict]);
		const delivered: number[] = [];
		const errors: string[] = [];
		const conflicts: string[] = [];
		const delays: number[] = [];
		await runInboxLoop({
			client: createClient(bus.fetchFn),
			getActive: () => ({ url: URL_, token: "tok" }),
			signal: new AbortController().signal,
			deliver: async e => void delivered.push(e.id),
			onError: error => void errors.push(error.message),
			onConflict: async error => void conflicts.push(error.code),
			sleep: async ms => void delays.push(ms),
		});
		expect(delivered).toEqual([1]);
		expect(conflicts).toEqual(["conflict", "conflict"]);
		expect(delays).toEqual([1000, 2000, 1000, 2000]);
		expect(errors).toEqual(["invalid token"]);
	});

	test("401 and 403 stay fatal", async () => {
		for (const [status, code] of [
			[401, "unauthenticated"],
			[403, "forbidden"],
		] as const) {
			const bus = fakeBus([() => errorResponse(status, code, "no"), () => new Response(streamOf([frame(entry(1))]))]);
			const delivered: number[] = [];
			await runInboxLoop({
				client: createClient(bus.fetchFn),
				getActive: () => ({ url: URL_, token: "tok" }),
				signal: new AbortController().signal,
				deliver: async e => void delivered.push(e.id),
				onError: () => {},
				onConflict: () => {},
				sleep: noSleep,
			});
			expect({ code, delivered, opens: bus.calls.length }).toEqual({ code, delivered: [], opens: 1 });
		}
	});
});

describe("conflictWarning", () => {
	const active = { url: URL_, token: "tok" };

	test("names the agent from whoami, and omits the name when whoami fails", async () => {
		const named = fakeBus([], { bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }) });
		expect(await conflictWarning(createClient(named.fetchFn), active)).toBe(
			"another session is already connected as w; inbox not delivered here",
		);
		const failing = fakeBus([], { bus_whoami: () => errorResponse(500, "internal", "boom") });
		expect(await conflictWarning(createClient(failing.fetchFn), active)).toBe(
			"another session is already connected as this agent; inbox not delivered here",
		);
	});
});

describe("client", () => {
	test("callTool posts JSON with bearer auth and unwraps result; errors become BusError", async () => {
		const bus = fakeBus([], {
			bus_whoami: () => Response.json({ result: { kind: "admin", spaces: [] } }),
			bus_fail: () => errorResponse(409, "conflict", "exists"),
		});
		const client = createClient(bus.fetchFn);
		expect(await client.callTool(URL_, "t", "bus_whoami", {})).toEqual({ kind: "admin", spaces: [] });
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_whoami`, method: "POST", auth: "Bearer t" });
		const error = await client.callTool(URL_, "t", "bus_fail", {}).catch(e => e);
		expect(error).toBeInstanceOf(BusError);
		expect(error).toMatchObject({ code: "conflict", message: "exists" });
	});
});

describe("registerRemoteTools", () => {
	type FakeTool = { name: string; label: string; execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }> };
	const listTool = (name: string, inputSchema: JsonSchema = { type: "object", properties: {} }): RemoteTool => ({ name, description: name, inputSchema });

	function setup(fetchFn: FetchLike, getActive: RemoteToolsOptions["getActive"] = () => ({ url: URL_, token: "t" })) {
		const tools: FakeTool[] = [];
		const warnings: string[] = [];
		const pi = { zod: z, registerTool: (t: FakeTool) => void tools.push(t) } as unknown as ExtensionAPI;
		const options: RemoteToolsOptions = {
			client: createClient(fetchFn),
			getActive,
			state: { registered: false, names: [] },
			warn: m => void warnings.push(m),
		};
		return { tools, warnings, options, register: () => registerRemoteTools(pi, options) };
	}

	test("registers once and executes with the current credentials", async () => {
		const seen: (string | undefined)[] = [];
		const fetchFn: FetchLike = async (url, init) => {
			if (url.endsWith("/v1/tools"))
				return Response.json([{ name: "bus_list_spaces", description: "List", inputSchema: { type: "object", properties: {}, additionalProperties: false } }]);
			seen.push(init.headers.Authorization);
			return Response.json({ result: [{ name: "demo" }] });
		};
		let token: string | undefined = "one";
		const { tools, register } = setup(fetchFn, () => (token ? { url: URL_, token } : undefined));
		expect(await register()).toBe(1);
		expect(await register()).toBe(0);
		expect(tools.map(t => [t.name, t.label])).toEqual([["bus_list_spaces", "bus_list_spaces"]]);
		token = undefined;
		await expect(tools[0].execute("id", {})).rejects.toThrow("Not logged in to the agent bus. Run /bus login <url> <token>.");
		token = "two";
		const out = await tools[0].execute("id", {});
		expect(out.content[0].text).toBe(JSON.stringify([{ name: "demo" }], null, 2));
		expect(seen).toEqual(["Bearer two"]);
	});

	test("a tool that fails to register is reported; the others still register", async () => {
		const list = [listTool("bus_a"), listTool("bus_bad", { type: "object", properties: { xs: { type: "array" } } }), listTool("bus_c")];
		const { tools, warnings, register } = setup(async () => Response.json(list));
		expect(await register()).toBe(2);
		expect(tools.map(t => t.name)).toEqual(["bus_a", "bus_c"]);
		expect(warnings).toEqual(["tool bus_bad not registered: unsupported schema type array"]);
	});

	test("a failed list fetch leaves registration open for the next attempt", async () => {
		let fail = true;
		const { tools, options, register } = setup(async () => (fail ? errorResponse(500, "internal", "boom") : Response.json([listTool("bus_a")])));
		await expect(register()).rejects.toThrow("boom");
		expect(options.state.registered).toBe(false);
		fail = false;
		expect(await register()).toBe(1);
		expect(options.state.registered).toBe(true);
		expect(tools.map(t => t.name)).toEqual(["bus_a"]);
	});

	test("excluded tools are not registered; registered names are recorded", async () => {
		const list = [listTool("bus_inbox_read"), listTool("bus_send"), listTool("bus_inbox_ack")];
		const { tools, options } = setup(async () => Response.json(list));
		expect(await registerRemoteTools({ zod: z, registerTool: (t: { name: string }) => void tools.push(t as never) } as unknown as ExtensionAPI, { ...options, exclude: INBOX_TOOLS })).toBe(1);
		expect(options.state.names).toEqual(["bus_send"]);
	});
});

function memoryStore(initial?: StoredCredentials): CredentialStore & { value: StoredCredentials | undefined } {
	const store = {
		value: initial,
		path: "mem",
		load: async () => store.value,
		save: async (c: StoredCredentials) => void (store.value = c),
		clear: async () => {
			const had = store.value !== undefined;
			store.value = undefined;
			return had;
		},
	};
	return store;
}

function commandDeps(
	store: CredentialStore,
	client: BusClient,
	session: SessionCredentials = {},
	server: ServerCli = async () => "",
): CommandDeps & { restarts: number; stops: number; activations: number } {
	const deps = {
		store,
		client,
		session,
		server,
		restarts: 0,
		stops: 0,
		activations: 0,
		restart: async () => void deps.restarts++,
		activate: async () => void deps.activations++,
		stop: async () => void deps.stops++,
	};
	return deps;
}

describe("runCommand", () => {
	const whoami = (byToken: Record<string, unknown>) => (_args: Record<string, unknown>, auth: string | undefined) =>
		Response.json({ result: byToken[auth?.replace("Bearer ", "") ?? ""] });

	test("login with an admin token stores adminToken and does not restart", async () => {
		const bus = fakeBus([], { bus_whoami: whoami({ adm: { kind: "admin", spaces: [] } }) });
		const store = memoryStore();
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const result = await runCommand(`login ${URL_}/ adm`, deps);
		expect(result).toEqual({ message: "Admin token stored. Run /bus promote <leader-name> to act as a leader.", level: "info" });
		expect(store.value).toEqual({ url: URL_, adminToken: "adm", token: undefined });
		expect({ restarts: deps.restarts, activations: deps.activations }).toEqual({ restarts: 0, activations: 1 });
	});

	test("login with a member token stores token and restarts; the admin token is kept only for the same url", async () => {
		const bus = fakeBus([], { bus_whoami: whoami({ mem: { kind: "member", name: "w", space: "demo", spaces: [{ name: "demo", role: "member" }] } }) });
		for (const [storedUrl, adminToken] of [
			["http://old", undefined],
			[URL_, "adm"],
		] as const) {
			const store = memoryStore({ url: storedUrl, adminToken: "adm" });
			const deps = commandDeps(store, createClient(bus.fetchFn));
			const result = await runCommand(["login", URL_, "mem"], deps);
			expect(result.message).toBe("Logged in as member w");
			expect({ storedUrl, value: store.value }).toEqual({ storedUrl, value: { url: URL_, adminToken, token: "mem" } });
			expect(store.value?.adminToken).toBe(adminToken);
			expect({ restarts: deps.restarts, activations: deps.activations }).toEqual({ restarts: 1, activations: 1 });
		}
	});

	test("promote without an admin token is an error", async () => {
		const deps = commandDeps(memoryStore({ url: URL_, token: "x" }), createClient(fakeBus([]).fetchFn));
		expect(await runCommand("promote lead", deps)).toEqual({ message: "Run /bus login <url> <admin-token> first.", level: "error" });
	});

	test("promote mints a leader with the admin token into the session only; the store is untouched", async () => {
		const bus = fakeBus([], { bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }) });
		const store = memoryStore({ url: `${URL_}/`, adminToken: "adm" });
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const result = await runCommand("promote lead", deps);
		expect(result).toEqual({
			message: "This session now acts as leader lead (until it ends; run /bus promote again after a restart).",
			level: "info",
		});
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_mint_leader`, auth: "Bearer adm", body: { name: "lead" } });
		expect(store.value).toEqual({ url: `${URL_}/`, adminToken: "adm" });
		expect(deps.session.override).toEqual({ url: URL_, token: "lead-tok" });
		expect(deps.restarts).toBe(1);
	});

	test("status shows the session's promoted identity over the stored one", async () => {
		const bus = fakeBus([], {
			bus_whoami: whoami({ "lead-tok": { kind: "leader", name: "lead", spaces: [] }, mem: { kind: "member", name: "w", spaces: [] } }),
		});
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), createClient(bus.fetchFn), { override: { url: URL_, token: "lead-tok" } });
		expect((await runCommand("status", deps)).message).toBe(`${URL_}: leader lead; spaces: none`);
	});

	test("logout clears the stored file, the session override and the env credentials with no fallback, and stops", async () => {
		const store = memoryStore({ url: URL_, adminToken: "adm", token: "t" });
		const deps = commandDeps(store, createClient(fakeBus([]).fetchFn), {
			override: { url: URL_, token: "lead-tok" },
			env: { url: URL_, token: "env" },
		});
		expect(await runCommand("logout", deps)).toEqual({ message: "Logged out; bus tools deactivated.", level: "info" });
		expect({ session: deps.session, stored: store.value }).toEqual({ session: {}, stored: undefined });
		expect({ restarts: deps.restarts, stops: deps.stops }).toEqual({ restarts: 0, stops: 1 });
	});

	test("login with an agent token clears the session override so the new token takes effect", async () => {
		const bus = fakeBus([], { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) });
		const deps = commandDeps(memoryStore(), createClient(bus.fetchFn), { override: { url: URL_, token: "lead-tok" } });
		await runCommand(`login ${URL_} mem`, deps);
		expect(deps.session.override).toBeUndefined();
		expect(deps.restarts).toBe(1);
	});

	test("status shows whoami; no credentials shows usage; logout clears; unknown is an error", async () => {
		const bus = fakeBus([], {
			bus_whoami: () => Response.json({ result: { kind: "leader", name: "lead", spaces: [{ name: "demo", role: "leader" }] } }),
		});
		const store = memoryStore({ url: URL_, token: "t" });
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const status = await runCommand("", deps);
		expect(status.level).toBe("info");
		expect(status.message).toContain("leader lead");
		expect(status.message).toContain("demo (leader)");
		expect(await runCommand("logout", deps)).toEqual({ message: "Logged out; bus tools deactivated.", level: "info" });
		expect(store.value).toBeUndefined();
		expect(deps.stops).toBe(1);
		expect((await runCommand("status", deps)).message).toContain("/bus login");
		expect((await runCommand("frobnicate", deps)).level).toBe("error");
	});

	test("help prints usage, which lists /bus help", async () => {
		const deps = commandDeps(memoryStore(), createClient(fakeBus([]).fetchFn));
		expect(await runCommand("help", deps)).toEqual({ message: USAGE, level: "info" });
		expect(USAGE).toContain("/bus help");
	});

	test("server errors surface as error messages", async () => {
		const bus = fakeBus([], { bus_whoami: () => errorResponse(401, "unauthenticated", "invalid token") });
		const deps = commandDeps(memoryStore(), createClient(bus.fetchFn));
		expect(await runCommand(`login ${URL_} bad`, deps)).toEqual({ message: "unauthenticated: invalid token", level: "error" });
	});
});

describe("credentials", () => {
	const dirs: string[] = [];
	afterEach(async () => {
		for (const dir of dirs.splice(0)) await rm(dir, { recursive: true, force: true });
	});

	test("resolveActive: session override > env credentials > file", () => {
		const stored = { url: "http://file", token: "file-tok" };
		const env = { url: "http://env", token: "env-tok" };
		const override = { url: "http://session", token: "session-tok" };
		expect(resolveActive({ override, env }, stored)).toEqual(override);
		expect(resolveActive({ env }, stored)).toEqual(env);
		expect(resolveActive({}, stored)).toEqual({ url: "http://file", token: "file-tok" });
		expect(resolveActive({}, { url: "http://file", adminToken: "a" })).toBeUndefined();
		expect(resolveActive({}, undefined)).toBeUndefined();
	});

	test("configDir: OMP_AGENT_BUS_DIR wins, else <PI_CODING_AGENT_DIR or ~/.omp/agent>/agent-message-bus", () => {
		expect(configDir({ OMP_AGENT_BUS_DIR: "/bus", PI_CODING_AGENT_DIR: "/agent" })).toBe("/bus");
		expect(configDir({ PI_CODING_AGENT_DIR: "/agent" })).toBe(join("/agent", "agent-message-bus"));
		expect(configDir({})).toBe(join(homedir(), ".omp", "agent", "agent-message-bus"));
	});

	test("store saves 0600, loads, clears; unreadable file is an error", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		const store = createCredentialStore({ OMP_AGENT_BUS_DIR: dir });
		expect(store.path).toBe(join(dir, "credentials.json"));
		expect(await store.load()).toBeUndefined();
		await store.save({ url: URL_, token: "t" });
		expect(await store.load()).toEqual({ url: URL_, token: "t" });
		if (process.platform !== "win32") expect((await stat(store.path)).mode & 0o777).toBe(0o600);
		expect(await store.clear()).toBe(true);
		expect(await store.clear()).toBe(false);
		await writeFile(store.path, "{nope");
		await expect(store.load()).rejects.toThrow(`Credential file ${store.path} is unreadable. Run /bus login to rewrite it.`);
	});
});

describe("index wiring", () => {
	const dirs: string[] = [];
	afterEach(async () => {
		for (const dir of dirs.splice(0)) await rm(dir, { recursive: true, force: true });
	});

	/**
	 * Boots the extension against a fake bus (default: entries 1 and 2 (critical)), runs session_start in `mode` until the
	 * scripted streams run out (the final 401 warning), and returns what was sent.
	 */
	async function runSession(
		mode: string,
		streams: (() => Response)[] = [() => new Response(streamOf([frame(entry(1)), frame(entry(2, { critical: true }))]))],
		tools: Parameters<typeof fakeBus>[1] = {},
	) {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		const bus = fakeBus(streams, tools);
		const sent: { message: { customType: string; content: string; display: boolean }; options: unknown }[] = [];
		const warnings: string[] = [];
		const { promise: warned, resolve: onWarn } = Promise.withResolvers<void>();
		const handlers: Record<string, ((event: unknown, ctx: { mode: string }) => unknown)[]> = {};
		const commands: { name: string; description: string }[] = [];
		const pi = {
			zod: z,
			registerTool: () => {},
			registerCommand: (name: string, options: { description: string }) => void commands.push({ name, description: options.description }),
			on: (event: string, handler: (event: unknown, ctx: { mode: string }) => unknown) => void (handlers[event] ??= []).push(handler),
			sendMessage: (message: (typeof sent)[number]["message"], options: unknown) => void sent.push({ message, options }),
			logger: {
				warn: (msg: string) => {
					warnings.push(msg);
					if (msg === "agent-bus: invalid token") onWarn();
				},
				info: () => {},
				error: () => {},
				debug: () => {},
			},
		} as unknown as ExtensionAPI;
		const extension = createBusExtension({
			fetchFn: bus.fetchFn,
			env: { OMP_AGENT_BUS_DIR: dir, AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" },
			sleep: noSleep,
		});
		await extension(pi);
		for (const handler of handlers.session_start ?? []) await handler({}, { mode });
		await warned;
		for (const handler of handlers.session_shutdown ?? []) await handler({}, { mode });
		return { sent, warnings, commands };
	}

	test("session_start streams the inbox: critical → steer, others → followUp", async () => {
		const { sent, warnings, commands } = await runSession("tui");
		expect(commands).toEqual([
			{ name: "bus", description: "Agent message bus: login <url> <token> | promote <leader-name> | server start|stop|status | status | logout | help" },
		]);
		expect(sent).toEqual([
			{ message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true }, options: { deliverAs: "followUp", triggerTurn: true } },
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { critical: true })), display: true },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
		expect(warnings).toEqual(["agent-bus: invalid token"]);
	});

	test("another session holding the stream warns once, retries, and delivers once the stream opens", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const { sent, warnings } = await runSession("tui", [conflict, conflict, () => new Response(streamOf([frame(entry(1))]))], {
			bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }),
		});
		expect(warnings).toEqual(["agent-bus: another session is already connected as w; inbox not delivered here", "agent-bus: invalid token"]);
		expect(sent.map(s => s.message.content)).toEqual([formatEntry(entry(1))]);
	});

	test("promote streams as the minted leader without writing its token to credentials.json; a new session forgets it", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		const env = { OMP_AGENT_BUS_DIR: dir };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, adminToken: "adm" });
		const bus = fakeBus([() => new Response(streamOf([frame(entry(1))]))], {
			bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }),
		});
		type Ctx = { mode: string; ui: { notify(message: string): void } };
		const handlers: Record<string, ((event: unknown, ctx: Ctx) => unknown)[]> = {};
		const commands: ((args: string, ctx: Ctx) => Promise<void>)[] = [];
		const { promise: ended, resolve: onEnd } = Promise.withResolvers<void>();
		const pi = {
			zod: z,
			registerTool: () => {},
			registerCommand: (_name: string, options: { handler: (typeof commands)[number] }) => void commands.push(options.handler),
			on: (event: string, handler: (event: unknown, ctx: Ctx) => unknown) => void (handlers[event] ??= []).push(handler),
			sendMessage: () => {},
			logger: { warn: () => onEnd(), info: () => {}, error: () => {}, debug: () => {} },
		} as unknown as ExtensionAPI;
		await createBusExtension({ fetchFn: bus.fetchFn, env, sleep: noSleep })(pi);
		const ctx: Ctx = { mode: "print", ui: { notify: () => {} } };
		const session = async (event: string) => {
			for (const handler of handlers[event] ?? []) await handler({}, ctx);
		};
		await session("session_start");
		await commands[0]("promote lead", ctx);
		await ended;
		await session("session_shutdown");
		await session("session_start");
		const streams = bus.calls.filter(c => c.url.endsWith("/v1/inbox/stream"));
		expect(streams.map(c => c.auth)).toEqual(["Bearer lead-tok", "Bearer lead-tok"]);
		expect(await store.load()).toEqual({ url: URL_, adminToken: "adm" });
	});

	test("only interactive modes (tui, rpc) trigger a turn; one-shot modes (print, json) do not", async () => {
		for (const [mode, triggerTurn] of [
			["print", false],
			["json", false],
			["tui", true],
			["rpc", true],
		] as const) {
			const { sent } = await runSession(mode);
			expect({ mode, options: sent.map(s => s.options) }).toEqual({
				mode,
				options: [
					{ deliverAs: "followUp", triggerTurn },
					{ deliverAs: "steer", triggerTurn },
				],
			});
		}
	});

	type Ctx = { mode: string; agent: { kind: string }; ui: { notify(message: string): void } };
	type BootTool = { name: string; execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }> };

	/** Loads the extension against a recording pi whose active tool set starts as `read` plus a foreign `bus_` tool. */
	async function boot(options: Parameters<typeof createBusExtension>[0], kind = "main") {
		const tools: BootTool[] = [];
		let active = ["read", "bus_foreign"];
		const handlers: Record<string, ((event: unknown, ctx: Ctx) => unknown)[]> = {};
		const commands: ((args: string, ctx: Ctx) => Promise<void>)[] = [];
		const notes: string[] = [];
		const pi = {
			zod: z,
			registerTool: (tool: BootTool) => {
				tools.push(tool);
				active.push(tool.name);
			},
			getActiveTools: () => [...active],
			setActiveTools: async (names: string[]) => void (active = names),
			registerCommand: (_name: string, opts: { handler: (typeof commands)[number] }) => void commands.push(opts.handler),
			on: (event: string, handler: (event: unknown, ctx: Ctx) => unknown) => void (handlers[event] ??= []).push(handler),
			sendMessage: () => {},
			logger: { warn: () => {}, info: () => {}, error: () => {}, debug: () => {} },
		} as unknown as ExtensionAPI;
		await createBusExtension({ sleep: noSleep, ...options })(pi);
		const ctx: Ctx = { mode: "print", agent: { kind }, ui: { notify: message => void notes.push(message) } };
		const emit = async (event: string) => {
			for (const handler of handlers[event] ?? []) await handler({}, ctx);
		};
		return { tools, notes, emit, active: () => active, run: (args: string) => commands[0](args, ctx) };
	}

	async function tempDir(): Promise<string> {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		return dir;
	}

	const listSpaces = { name: "bus_list_spaces", description: "List", inputSchema: { type: "object" as const, properties: {} } };
	const memberBus = () =>
		fakeBus(
			[],
			{
				bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }),
				bus_list_spaces: (_args, auth) => Response.json({ result: auth }),
			},
			[listSpaces],
		);

	test("logout deactivates every bus_* tool and keeps the others; calls fail until login re-activates them with the new token", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: URL_, token: "mem" });
		const app = await boot({ fetchFn: memberBus().fetchFn, env });
		await app.emit("session_start");
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_list_spaces"]);
		await app.run("logout");
		expect(app.active()).toEqual(["read"]);
		await expect(app.tools[0].execute("id", {})).rejects.toThrow("Not logged in to the agent bus. Run /bus login <url> <token>.");
		await app.run(`login ${URL_} mem2`);
		expect(app.active()).toEqual(["read", "bus_list_spaces"]);
		const out = await app.tools[0].execute("id", {});
		await app.emit("session_shutdown");
		expect(out.content[0].text).toBe(JSON.stringify("Bearer mem2", null, 2));
		expect(app.tools).toHaveLength(1);
		expect(app.notes).toEqual(["Logged out; bus tools deactivated.", "Logged in as member w"]);
	});

	test("env credentials are deleted from the environment at load, used from memory, and logout leaves no env fallback", async () => {
		const env: NodeJS.ProcessEnv = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: `${URL_}/`, AGENT_BUS_TOKEN: "env-tok", AGENT_BUS_BIN: "/opt/amb" };
		const bus = memberBus();
		const app = await boot({ fetchFn: bus.fetchFn, env });
		expect(env).toEqual({ OMP_AGENT_BUS_DIR: env.OMP_AGENT_BUS_DIR });
		await app.emit("session_start");
		expect(bus.calls.find(c => c.url.endsWith("/v1/tools"))?.auth).toBe("Bearer env-tok");
		await app.run("logout");
		await expect(app.tools[0].execute("id", {})).rejects.toThrow("Not logged in");
		await app.run("status");
		expect(app.notes[1]).toStartWith("Not logged in.");
	});

	test("the default environment is process.env, and the bus variables are deleted from it at load", async () => {
		const saved = { ...process.env };
		try {
			Object.assign(process.env, { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok", AGENT_BUS_BIN: "/opt/amb" });
			const bus = memberBus();
			const app = await boot({ fetchFn: bus.fetchFn });
			expect([process.env.AGENT_BUS_URL, process.env.AGENT_BUS_TOKEN, process.env.AGENT_BUS_BIN]).toEqual([undefined, undefined, undefined]);
			await app.emit("session_start");
			expect(bus.calls.find(c => c.url.endsWith("/v1/tools"))?.auth).toBe("Bearer env-tok");
			await app.emit("session_shutdown");
		} finally {
			for (const key of Object.keys(process.env)) if (!(key in saved)) delete process.env[key];
			Object.assign(process.env, saved);
		}
	});

	test("a subagent session gets no inbox loop and no inbox tools; the other bus tools register; a main session gets both", async () => {
		const schema = { type: "object" as const, properties: {} };
		const list = ["bus_inbox_read", "bus_send", "bus_inbox_ack"].map(name => ({ name, description: name, inputSchema: schema }));
		const results: Record<string, { tools: string[]; streamed: boolean }> = {};
		for (const kind of ["sub", "main"]) {
			const bus = fakeBus([], {}, list);
			const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" } }, kind);
			await app.emit("session_start");
			await app.emit("session_shutdown");
			results[kind] = { tools: app.tools.map(t => t.name), streamed: bus.calls.some(c => c.url.endsWith("/v1/inbox/stream")) };
		}
		expect(results).toEqual({
			sub: { tools: ["bus_send"], streamed: false },
			main: { tools: ["bus_inbox_read", "bus_send", "bus_inbox_ack"], streamed: true },
		});
	});

	/** Fake agent-message-bus CLI: records each run and answers start, status --json, admin-token, stop and status. */
	function fakeCli(running = true) {
		const runs: string[][] = [];
		const out: Record<string, string> = {
			start: "agent-message-bus started (pid 7) at http://127.0.0.1:8080; log: /x.log\n",
			"status --json": JSON.stringify({ running, pid: 7, url: "http://127.0.0.1:8080" }),
			"admin-token": "amb_admin_secret\n",
			stop: "agent-message-bus stopped (pid 7)\n",
			status: "running (pid 7) at http://127.0.0.1:8080\n",
		};
		const runner: Runner = async (command, args) => {
			runs.push([command, ...args]);
			return { code: 0, stdout: out[args.join(" ")] ?? "", stderr: "" };
		};
		return { runs, runner };
	}

	test("/bus server start runs start, status --json, admin-token; stores the admin credentials and re-activates the bus tools", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_BIN: "/opt/amb" };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, token: "mem" });
		const cli = fakeCli();
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: cli.runner, which: async () => "/usr/bin/agent-message-bus" });
		await app.emit("session_start");
		await app.run("logout");
		await app.run("server start");
		expect(cli.runs).toEqual([
			["/opt/amb", "start"],
			["/opt/amb", "status", "--json"],
			["/opt/amb", "admin-token"],
		]);
		expect(await store.load()).toEqual({ url: "http://127.0.0.1:8080", adminToken: "amb_admin_secret" });
		expect(app.active()).toEqual(["read", "bus_list_spaces"]);
		expect(app.notes[1]).toBe(
			"Agent message bus running at http://127.0.0.1:8080; admin token stored. Run /bus promote <leader-name> to act as a leader.",
		);
	});

	test("/bus server stop runs stop and logs out; /bus server status prints the CLI status", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, token: "mem" });
		const cli = fakeCli();
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: cli.runner, which: async () => "/usr/bin/agent-message-bus" });
		await app.emit("session_start");
		await app.run("server stop");
		await app.run("server status");
		expect(cli.runs).toEqual([
			["/usr/bin/agent-message-bus", "stop"],
			["/usr/bin/agent-message-bus", "status"],
		]);
		expect(await store.load()).toBeUndefined();
		expect(app.active()).toEqual(["read"]);
		expect(app.notes).toEqual(["agent-message-bus stopped (pid 7)\nLogged out; bus tools deactivated.", "running (pid 7) at http://127.0.0.1:8080"]);
	});

	test("/bus server start fails when the CLI reports no running instance, or without a subcommand shows its usage", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_BIN: "/opt/amb" };
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: fakeCli(false).runner });
		await app.run("server start");
		await app.run("server");
		expect(app.notes).toEqual(["agent-message-bus started but reports no running instance", "Usage: /bus server start|stop|status"]);
		expect(await createCredentialStore(env).load()).toBeUndefined();
	});
});

describe("consumeEnv", () => {
	test("copies url, token and bin into memory and deletes them; credentials need both url and token", () => {
		for (const [input, expected] of [
			[{ AGENT_BUS_URL: "http://a/", AGENT_BUS_TOKEN: "t", AGENT_BUS_BIN: "/b", PATH: "/p" }, { credentials: { url: "http://a", token: "t" }, bin: "/b" }],
			[{ AGENT_BUS_URL: "http://a", PATH: "/p" }, {}],
			[{ PATH: "/p" }, {}],
		] as const) {
			const env: NodeJS.ProcessEnv = { ...input };
			expect(consumeEnv(env)).toEqual(expected);
			expect(env).toEqual({ PATH: "/p" });
		}
	});
});

describe("activation", () => {
	test("deactivate drops every bus_* tool; activate re-adds the missing names only", async () => {
		let active = ["read", "bus_a", "edit", "bus_b"];
		const pi = { getActiveTools: () => [...active], setActiveTools: async (names: string[]) => void (active = names) };
		await deactivateBusTools(pi);
		expect(active).toEqual(["read", "edit"]);
		await activateBusTools(pi, ["bus_a", "bus_b"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
		await activateBusTools(pi, ["bus_a"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
	});
});

describe("server cli", () => {
	test("runs the resolved binary and returns stdout; a non-zero exit is an error with stderr", async () => {
		const runner: Runner = async (_command, args) =>
			args[0] === "ok" ? { code: 0, stdout: "out", stderr: "" } : { code: 1, stdout: "", stderr: "boom\n" };
		const cli = createServerCli(runner, async () => "/bin/amb");
		expect(await cli(["ok"])).toBe("out");
		await expect(cli(["stop"])).rejects.toThrow("agent-message-bus stop failed (exit 1): boom");
	});
});

describe("binary resolution", () => {
	const dirs: string[] = [];
	afterEach(async () => {
		for (const dir of dirs.splice(0)) await rm(dir, { recursive: true, force: true });
	});

	const BYTES = "fake-binary";
	const sha = (text: string) => createHash("sha256").update(text).digest("hex");
	const ASSET = "agent-message-bus_1.2.3_linux_amd64";

	/** Fake GitHub: latest release amb-v1.2.3 with the linux/amd64 asset and SHA256SUMS listing `sum`. */
	function fakeGitHub(sum = sha(BYTES)) {
		const urls: string[] = [];
		const fetchFn: FetchLike = async url => {
			urls.push(url);
			if (url === RELEASES_URL)
				return Response.json({
					tag_name: "amb-v1.2.3",
					assets: [
						{ name: ASSET, browser_download_url: "https://dl/asset" },
						{ name: "SHA256SUMS", browser_download_url: "https://dl/sums" },
					],
				});
			if (url === "https://dl/sums") return new Response(`${sha("other")}  agent-message-bus_1.2.3_darwin_arm64\n${sum}  ${ASSET}\n`);
			if (url === "https://dl/asset") return new Response(BYTES);
			return new Response("missing", { status: 404 });
		};
		return { urls, fetchFn };
	}

	async function options(fetchFn: FetchLike, extra: { bin?: string; onPath?: string } = {}) {
		const dir = await mkdtemp(join(tmpdir(), "amb-bin-"));
		dirs.push(dir);
		return { bin: extra.bin, which: async () => extra.onPath, fetchFn, dir, platform: "linux", arch: "x64" };
	}

	test("AGENT_BUS_BIN wins over PATH; PATH wins over a download", async () => {
		const gh = fakeGitHub();
		expect(await resolveBinary(await options(gh.fetchFn, { bin: "/env/amb", onPath: "/usr/bin/agent-message-bus" }))).toBe("/env/amb");
		expect(await resolveBinary(await options(gh.fetchFn, { onPath: "/usr/bin/agent-message-bus" }))).toBe("/usr/bin/agent-message-bus");
		expect(gh.urls).toEqual([]);
	});

	test("downloads the latest release asset, verifies its sha256, caches it under <dir>/<version>/ and reuses the cache", async () => {
		const gh = fakeGitHub();
		const opts = await options(gh.fetchFn);
		const path = await resolveBinary(opts);
		expect(path).toBe(join(opts.dir, "1.2.3", "agent-message-bus"));
		expect(await readFile(path, "utf8")).toBe(BYTES);
		if (process.platform !== "win32") expect((await stat(path)).mode & 0o777).toBe(0o755);
		expect(await resolveBinary(opts)).toBe(path);
		expect(gh.urls).toEqual([RELEASES_URL, "https://dl/sums", "https://dl/asset", RELEASES_URL]);
	});

	test("a checksum mismatch is an error and nothing is cached", async () => {
		const opts = await options(fakeGitHub(sha("tampered")).fetchFn);
		await expect(resolveBinary(opts)).rejects.toThrow(`checksum mismatch for ${ASSET}`);
		await expect(stat(join(opts.dir, "1.2.3"))).rejects.toThrow();
	});

	test("a failed release lookup is an error", async () => {
		const opts = await options(async () => new Response("rate limited", { status: 403 }));
		await expect(resolveBinary(opts)).rejects.toThrow(`download ${RELEASES_URL} failed: HTTP 403`);
	});

	test("assetName maps platform and arch to the release naming; unsupported platforms are an error", () => {
		for (const [platform, arch, name] of [
			["win32", "x64", "agent-message-bus_1.0.0_windows_amd64.exe"],
			["linux", "arm64", "agent-message-bus_1.0.0_linux_arm64"],
			["darwin", "x64", "agent-message-bus_1.0.0_darwin_amd64"],
		] as const)
			expect(assetName("1.0.0", platform, arch)).toBe(name);
		expect(() => assetName("1.0.0", "aix", "ppc64")).toThrow("no agent-message-bus release for aix/ppc64");
	});

	test("expectedSum reads text and binary-mode SHA256SUMS lines; a missing entry is an error", () => {
		expect(expectedSum("ABC  a\ndef *b\n", "a")).toBe("abc");
		expect(expectedSum("ABC  a\ndef *b\n", "b")).toBe("def");
		expect(() => expectedSum("abc  a\n", "c")).toThrow("SHA256SUMS has no entry for c");
	});
});
