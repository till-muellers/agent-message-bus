/** Shared test doubles: the recording `pi.zod`, the fake bus server, an in-memory credential store and the one fake `pi`. */
import { afterEach } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { type BusExtensionOptions, createBusExtension } from "../extension/index.ts";
import type { FetchLike, RemoteTool } from "../extension/lib/bus/client.ts";
import type { CredentialStore, StoredCredentials } from "../extension/lib/credentials.ts";
import type { Entry } from "../extension/lib/inbox.ts";
import type { JsonSchema, Zod } from "../extension/lib/bus/schema.ts";

/**
 * Recording stand-in for `pi.zod` (no zod package in this repo): nodes keep enough state to rebuild JSON schema.
 * Mirrors omp's omptype-backed shim surface: no `z.strictObject` (regression), objects use `.strict()`.
 */
export interface FakeNode {
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
	return {
		...fields,
		int: () => fakeNode({ ...fields, type: "integer" }),
		optional: () => fakeNode({ ...fields, isOptional: true }),
		describe: description => fakeNode({ ...fields, description }),
		strict: () => fakeNode({ ...fields, isStrict: true }),
	} as FakeNode;
}

export const fakeZ = {
	string: () => fakeNode({ type: "string" }),
	number: () => fakeNode({ type: "number" }),
	boolean: () => fakeNode({ type: "boolean" }),
	object: (shape: Record<string, FakeNode>) => fakeNode({ type: "object", shape }),
};
export const z = fakeZ as unknown as Zod;

export function toJson(node: FakeNode): JsonSchema {
	const out: JsonSchema = { type: node.type };
	if (node.description) out.description = node.description;
	if (node.shape) {
		out.properties = Object.fromEntries(Object.entries(node.shape).map(([k, v]) => [k, toJson(v)]));
		out.required = Object.entries(node.shape)
			.filter(([, v]) => v.isOptional !== true)
			.map(([k]) => k);
		if (node.isStrict) out.additionalProperties = false;
	}
	return out;
}

export const URL_ = "http://bus.test";
export const CODE = "k7m2-x9pq";

export function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
	const encoder = new TextEncoder();
	return new ReadableStream({
		start(controller) {
			for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
			controller.close();
		},
	});
}

export function entry(id: number, extra: Partial<Entry> = {}): Entry {
	return { id, space: "demo", type: "direct_message", from: "lead", critical: false, body: `body ${id}`, createdAt: "t", ...extra };
}

export function frame(e: Entry): string {
	return `id: ${e.id}\nevent: inbox\ndata: ${JSON.stringify(e)}\n\n`;
}

export function errorResponse(status: number, code: string, message: string): Response {
	return Response.json({ error: { code, message } }, { status });
}

export interface Call {
	url: string;
	method: string;
	auth: string | undefined;
	body: unknown;
}

export type ToolHandler = (args: Record<string, unknown>, auth: string | undefined) => Response;

export interface FakeBusOptions {
	/** Each inbox stream open shifts the next response; none left answers 401 `invalid token`. */
	streams?: (() => Response)[];
	/** `POST /v1/tools/<name>` handlers; unknown tools answer `{result: {acked: 1}}`. */
	tools?: Record<string, ToolHandler>;
	/** What `GET /v1/tools` lists. */
	list?: RemoteTool[];
	/** The first prompt `POST /v1/invites/accept` returns with the member token; omitted when undefined. */
	task?: string;
}

/** The fake server's `POST /v1/invites/accept`: `CODE` (any case and dashes) admits member worker of demo with `acc-tok`. */
function acceptInviteResponse(auth: string | undefined, task: string | undefined): Response {
	const code = (auth ?? "").replace("Bearer ", "").toLowerCase().replace(/[^a-z0-9]/g, "");
	if (code !== CODE.replace("-", "")) return errorResponse(401, "unauthenticated", "invalid or expired invite code");
	return Response.json({ name: "worker", space: "demo", token: "acc-tok", ...(task === undefined ? {} : { task }) });
}

/** Fake bus server following the HTTP contract; records every call. */
export function fakeBus({ streams = [], tools = {}, list = [], task }: FakeBusOptions = {}) {
	const calls: Call[] = [];
	const fetchFn: FetchLike = async (url, init) => {
		const body = init.body ? JSON.parse(init.body) : undefined;
		calls.push({ url, method: init.method, auth: init.headers.Authorization, body });
		if (url.endsWith("/v1/inbox/stream")) return streams.shift()?.() ?? errorResponse(401, "unauthenticated", "invalid token");
		if (url.endsWith("/v1/tools")) return Response.json(list);
		if (url.endsWith("/v1/invites/accept")) return acceptInviteResponse(init.headers.Authorization, task);
		const handler = tools[url.slice(url.lastIndexOf("/") + 1)];
		return handler ? handler(body ?? {}, init.headers.Authorization) : Response.json({ result: { acked: 1 } });
	};
	return { calls, fetchFn };
}

export const listSpaces: RemoteTool = { name: "bus_list_spaces", description: "List", inputSchema: { type: "object", properties: {} } };

/** A bus whose token is member w and whose only tool, bus_list_spaces, echoes the caller's bearer. */
export function memberBus(options: FakeBusOptions = {}) {
	return fakeBus({
		tools: {
			bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }),
			bus_list_spaces: (_args, auth) => Response.json({ result: auth }),
		},
		list: [listSpaces],
		...options,
	});
}

export const callsTo = (bus: { calls: Call[] }, suffix: string) => bus.calls.filter(c => c.url.endsWith(suffix));

export const noSleep = async (): Promise<void> => {};

export function memoryStore(initial?: StoredCredentials): CredentialStore & { value: StoredCredentials | undefined } {
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

/** A temp-dir factory for the calling `describe`; the dirs are removed after each test. */
export function useTempDirs(): () => Promise<string> {
	const dirs: string[] = [];
	afterEach(async () => {
		for (const dir of dirs.splice(0)) await rm(dir, { recursive: true, force: true });
	});
	return async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-"));
		dirs.push(dir);
		return dir;
	};
}

export interface BootTool {
	name: string;
	description: string;
	parameters: unknown;
	defaultInactive?: boolean;
	execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }>;
}

/** Per-session inputs of the fake omp; mutate them to model a session switch. */
export interface BootSession {
	kind?: string;
	mode?: string;
	cwd?: string;
	flags?: Record<string, string>;
	sessionFile?: string;
}

export interface Sent {
	message: { customType: string; content: string; display: boolean };
	options: unknown;
}

interface Ctx {
	mode: string;
	cwd: string;
	agent: { kind: string };
	ui: { notify(message: string): void };
	sessionManager: { getSessionFile(): string | undefined };
}

type Handler = (event: unknown, ctx: Ctx) => unknown;

function sessionContext(session: BootSession, notes: string[]): Ctx {
	return {
		get mode() {
			return session.mode ?? "print";
		},
		get cwd() {
			return session.cwd ?? "/w";
		},
		get agent() {
			return { kind: session.kind ?? "main" };
		},
		ui: { notify: message => void notes.push(message) },
		sessionManager: { getSessionFile: () => session.sessionFile },
	};
}

/** Records what a logger receives; `warning(m)` resolves once `agent-bus: m` was logged. */
function recordingLogger() {
	const warnings: string[] = [];
	const waiters: { message: string; resolve(): void }[] = [];
	const warn = (message: string) => {
		warnings.push(message);
		for (const waiter of waiters) if (waiter.message === message) waiter.resolve();
	};
	const warning = (text: string): Promise<void> => {
		const message = `agent-bus: ${text}`;
		if (warnings.includes(message)) return Promise.resolve();
		const { promise, resolve } = Promise.withResolvers<void>();
		waiters.push({ message, resolve });
		return promise;
	};
	return { warnings, warning, logger: { warn, info: () => {}, error: () => {}, debug: () => {} } };
}

/**
 * The one fake omp: loads the extension against a recording `pi` whose active tool set starts as `read` plus a foreign
 * `bus_` tool. Tools registered with `defaultInactive` stay out of the active set until the extension activates them.
 */
export async function boot(options: BusExtensionOptions, session: BootSession = {}) {
	const tools: BootTool[] = [];
	let active = ["read", "bus_foreign"];
	const handlers: Record<string, Handler[]> = {};
	const commands: { description: string; handler: (args: string, ctx: Ctx) => Promise<void> }[] = [];
	const notes: string[] = [];
	const userMessages: string[] = [];
	const sent: Sent[] = [];
	const { warnings, warning, logger } = recordingLogger();
	const pi = {
		zod: z,
		registerTool: (tool: BootTool) => {
			tools.push(tool);
			if (!tool.defaultInactive) active.push(tool.name);
		},
		registerFlag: () => {},
		getFlag: (name: string) => session.flags?.[name],
		sendUserMessage: (text: string) => void userMessages.push(text),
		sendMessage: (message: Sent["message"], options: unknown) => void sent.push({ message, options }),
		getActiveTools: () => [...active],
		setActiveTools: async (names: string[]) => void (active = names),
		registerCommand: (_name: string, command: (typeof commands)[number]) => void commands.push(command),
		on: (event: string, handler: Handler) => void (handlers[event] ??= []).push(handler),
		logger,
	} as unknown as ExtensionAPI;
	await createBusExtension({ sleep: noSleep, ...options })(pi);
	const ctx = sessionContext(session, notes);
	const emit = async (event: string, payload: unknown = { type: event }) => {
		for (const handler of handlers[event] ?? []) await handler(payload, ctx);
	};
	const tool = (name: string): BootTool => {
		const found = tools.find(t => t.name === name);
		if (!found) throw new Error(`tool ${name} not registered`);
		return found;
	};
	const run = (args: string) => commands[0].handler(args, ctx);
	return { tools, tool, notes, warnings, warning, userMessages, sent, emit, run, commands, active: () => active };
}
