import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { type JsonSchema, toZod, type Zod } from "../extensions/omp/agent-message-bus/lib/schema.ts";
import { createBusExtension } from "../extensions/omp/agent-message-bus/index.ts";
import { BusError, type BusClient, createClient, type FetchLike } from "../extensions/omp/agent-message-bus/lib/client.ts";
import { runCommand, type CommandDeps } from "../extensions/omp/agent-message-bus/lib/command.ts";
import {
	type CredentialStore,
	createCredentialStore,
	resolveActive,
	type StoredCredentials,
} from "../extensions/omp/agent-message-bus/lib/credentials.ts";
import { type Entry, formatEntry, runInboxLoop } from "../extensions/omp/agent-message-bus/lib/inbox.ts";
import { parseSse } from "../extensions/omp/agent-message-bus/lib/sse.ts";
import { registerRemoteTools } from "../extensions/omp/agent-message-bus/lib/tools.ts";

/** Recording stand-in for `pi.zod` (no zod package in this repo): nodes keep enough state to rebuild JSON schema. */
interface FakeNode {
	type: string;
	description?: string;
	isOptional?: boolean;
	shape?: Record<string, FakeNode>;
	strict?: boolean;
	int(): FakeNode;
	optional(): FakeNode;
	describe(text: string): FakeNode;
}
function fakeNode(fields: Partial<FakeNode> & { type: string }): FakeNode {
	const node: FakeNode = {
		...fields,
		int: () => fakeNode({ ...fields, type: "integer" }),
		optional: () => fakeNode({ ...fields, isOptional: true }),
		describe: description => fakeNode({ ...fields, description }),
	} as FakeNode;
	return node;
}
const fakeZ = {
	string: () => fakeNode({ type: "string" }),
	number: () => fakeNode({ type: "number" }),
	boolean: () => fakeNode({ type: "boolean" }),
	object: (shape: Record<string, FakeNode>) => fakeNode({ type: "object", shape }),
	strictObject: (shape: Record<string, FakeNode>) => fakeNode({ type: "object", shape, strict: true }),
};
const z = fakeZ as unknown as Zod;

function toJson(node: FakeNode): JsonSchema {
	const out: JsonSchema = { type: node.type };
	if (node.description) out.description = node.description;
	if (node.shape) {
		out.properties = Object.fromEntries(Object.entries(node.shape).map(([k, v]) => [k, toJson(v)]));
		out.required = Object.entries(node.shape).filter(([, v]) => v.isOptional !== true).map(([k]) => k);
		if (node.strict) out.additionalProperties = false;
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

/** Fake bus: each stream open shifts the next scripted response; tool calls go to `tools`. */
function fakeBus(streams: (() => Response)[], tools: Record<string, (args: Record<string, unknown>, auth: string | undefined) => Response> = {}) {
	const calls: Call[] = [];
	const fetchFn: FetchLike = async (url, init) => {
		calls.push({ url, method: init.method, auth: init.headers.Authorization, body: init.body ? JSON.parse(init.body) : undefined });
		if (url.endsWith("/v1/inbox/stream")) {
			const next = streams.shift();
			return next ? next() : errorResponse(401, "unauthenticated", "invalid token");
		}
		if (url.endsWith("/v1/tools")) return Response.json([]);
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
		expect(toJson(toZod(schema, z) as unknown as FakeNode)).toEqual(schema);
	});

	test("optional fields keep their description", () => {
		const node = toZod({ type: "object", properties: { a: { type: "string", description: "d" } } }, z) as unknown as FakeNode;
		expect(node.shape?.a).toMatchObject({ type: "string", description: "d", isOptional: true });
		expect(node.strict).toBeUndefined();
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
			sleep: noSleep,
		});
		expect(bus.calls).toHaveLength(0);
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
	test("registers once and executes with the current credentials", async () => {
		const seen: (string | undefined)[] = [];
		const fetchFn: FetchLike = async (url, init) => {
			if (url.endsWith("/v1/tools"))
				return Response.json([{ name: "bus_list_spaces", description: "List", inputSchema: { type: "object", properties: {}, additionalProperties: false } }]);
			seen.push(init.headers.Authorization);
			if (url.endsWith("/bus_fail")) return errorResponse(403, "forbidden", "nope");
			return Response.json({ result: [{ name: "demo" }] });
		};
		const tools: { name: string; label: string; execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }> }[] = [];
		const pi = { zod: z, registerTool: (t: (typeof tools)[number]) => void tools.push(t) } as unknown as ExtensionAPI;
		let token = "one";
		const state = { registered: false };
		const getActive = () => ({ url: URL_, token });
		await registerRemoteTools(pi, createClient(fetchFn), getActive, state);
		await registerRemoteTools(pi, createClient(fetchFn), getActive, state);
		expect(tools.map(t => [t.name, t.label])).toEqual([["bus_list_spaces", "bus_list_spaces"]]);
		token = "two";
		const out = await tools[0].execute("id", {});
		expect(out.content[0].text).toBe(JSON.stringify([{ name: "demo" }], null, 2));
		expect(seen).toEqual(["Bearer two"]);
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

function commandDeps(store: CredentialStore, client: BusClient): CommandDeps & { restarts: number; stops: number } {
	const deps = {
		store,
		client,
		env: {},
		restarts: 0,
		stops: 0,
		restart: async () => void deps.restarts++,
		stop: () => void deps.stops++,
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
		expect(deps.restarts).toBe(0);
	});

	test("login with a member token stores token and restarts", async () => {
		const bus = fakeBus([], { bus_whoami: whoami({ mem: { kind: "member", name: "w", space: "demo", spaces: [{ name: "demo", role: "member" }] } }) });
		const store = memoryStore({ url: "http://old", adminToken: "adm" });
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const result = await runCommand(["login", URL_, "mem"], deps);
		expect(result.message).toBe("Logged in as member w");
		expect(store.value).toEqual({ url: URL_, adminToken: "adm", token: "mem" });
		expect(deps.restarts).toBe(1);
	});

	test("promote without an admin token is an error", async () => {
		const deps = commandDeps(memoryStore({ url: URL_, token: "x" }), createClient(fakeBus([]).fetchFn));
		expect(await runCommand("promote lead", deps)).toEqual({ message: "Run /bus login <url> <admin-token> first.", level: "error" });
	});

	test("promote mints a leader with the admin token and stores it", async () => {
		const bus = fakeBus([], { bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }) });
		const store = memoryStore({ url: URL_, adminToken: "adm" });
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const result = await runCommand("promote lead", deps);
		expect(result).toEqual({ message: "This session now acts as leader lead.", level: "info" });
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_mint_leader`, auth: "Bearer adm", body: { name: "lead" } });
		expect(store.value).toEqual({ url: URL_, adminToken: "adm", token: "lead-tok" });
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
		expect(await runCommand("logout", deps)).toEqual({ message: "Logged out.", level: "info" });
		expect(store.value).toBeUndefined();
		expect(deps.stops).toBe(1);
		expect((await runCommand("status", deps)).message).toContain("/bus login");
		expect((await runCommand("frobnicate", deps)).level).toBe("error");
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

	test("resolveActive: env overrides the file", () => {
		const stored = { url: "http://file", token: "file-tok" };
		expect(resolveActive({}, stored)).toEqual({ url: "http://file", token: "file-tok" });
		expect(resolveActive({ AGENT_BUS_URL: "http://env/", AGENT_BUS_TOKEN: "env-tok" }, stored)).toEqual({ url: "http://env", token: "env-tok" });
		expect(resolveActive({ AGENT_BUS_URL: "http://env" }, stored)).toEqual({ url: "http://file", token: "file-tok" });
		expect(resolveActive({}, { url: "http://file", adminToken: "a" })).toBeUndefined();
		expect(resolveActive({}, undefined)).toBeUndefined();
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

	test("session_start streams the inbox: critical → steer, others → followUp", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		const bus = fakeBus([() => new Response(streamOf([frame(entry(1)), frame(entry(2, { critical: true }))]))]);
		const sent: { message: { customType: string; content: string; display: boolean }; options: unknown }[] = [];
		const warnings: string[] = [];
		const { promise: warned, resolve: onWarn } = Promise.withResolvers<void>();
		const handlers: Record<string, (() => unknown)[]> = {};
		const commands: string[] = [];
		const pi = {
			zod: z,
			registerTool: () => {},
			registerCommand: (name: string) => void commands.push(name),
			on: (event: string, handler: () => unknown) => void (handlers[event] ??= []).push(handler),
			sendMessage: (message: (typeof sent)[number]["message"], options: unknown) => void sent.push({ message, options }),
			logger: {
				warn: (msg: string) => {
					warnings.push(msg);
					onWarn();
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
		expect(commands).toEqual(["bus"]);
		for (const handler of handlers.session_start ?? []) await handler();
		await warned;
		expect(sent).toEqual([
			{ message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true }, options: { deliverAs: "followUp", triggerTurn: true } },
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { critical: true })), display: true },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
		expect(warnings).toEqual(["agent-bus: invalid token"]);
		for (const handler of handlers.session_shutdown ?? []) await handler();
	});
});
