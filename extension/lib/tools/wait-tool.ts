/**
 * `bus_wait` in omp: a local tool with the server tool's name, description and schema (`docs/design/arch/client.md` §3.4).
 * With the session's own inbox stream it returns the entries the dispatcher still holds at once, else it waits in the
 * dispatcher, so the entries it returns never reach omp a second time and count as consumed; without one it forwards the
 * call to the server's `bus_wait`, which acks nothing. What a forwarded call returns is claimed from the dispatcher: an
 * entry the session's stream delivered meanwhile is left out, and one it delivers later is skipped there. When that
 * leaves nothing, the call waits on after the newest entry for the time left (in the dispatcher once the session's stream
 * is open, else at the server), so [] still means timeout.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { BusError, type RemoteTool } from "../bus/client.ts";
import type { Entry } from "../bus/entry.ts";
import { toZod } from "../bus/schema.ts";
import type { Dispatcher } from "../delivery/dispatcher.ts";
import type { WaitFilter } from "../delivery/waiters.ts";
import { presentationOf } from "./presentation.ts";
import { type ToolResult, toolResult } from "./tool-result.ts";

export const WAIT_TOOL = "bus_wait";

/** The arguments of `bus_wait` (contracts.md §2.5). */
export interface WaitArgs extends WaitFilter {
	timeoutSeconds?: number;
}

/** What `bus_wait` uses of the dispatcher of the session's open inbox stream. */
type LocalWait = Pick<Dispatcher, "takeHeld" | "waitNext">;

export interface WaitToolDeps {
	/** The dispatcher of the session's open inbox stream; undefined while the session holds none. */
	dispatcher(): LocalWait | undefined;
	/** Calls the server's `bus_wait` with a transport timeout. */
	forward(args: WaitArgs, timeoutMs: number, signal?: AbortSignal): Promise<unknown>;
	/** The entries a forwarded call returned that the session's stream has not delivered (`Dispatcher.claim`). */
	claim(entries: readonly Entry[]): Entry[];
	/** The ids the tool returned, from the stream or claimed from a forwarded call: consumed. */
	consumed(ids: readonly number[]): void;
}

const DEFAULT_SECONDS = 30;
const MAX_SECONDS = 300;
/** The transport outlives the server's wait by this margin (contracts.md §2.5, CR-7). */
const TRANSPORT_MARGIN_MS = 10_000;

function waitSeconds({ timeoutSeconds = DEFAULT_SECONDS }: WaitArgs): number {
	if (timeoutSeconds < 1 || timeoutSeconds > MAX_SECONDS) throw new BusError("invalid", `timeoutSeconds must be 1..${MAX_SECONDS}`);
	return timeoutSeconds;
}

const isEntry = (value: unknown): value is Entry => typeof (value as { id?: unknown } | null)?.id === "number";

/** The entries the tool returns, consumed. */
function consumed(entries: Entry[], deps: WaitToolDeps): Entry[] {
	if (entries.length > 0) deps.consumed(entries.map(entry => entry.id));
	return entries;
}

/** In the dispatcher: the held entries matching `args` at once, else the next one within `ms`; consumed. */
async function local(dispatcher: LocalWait, args: WaitArgs, ms: number, deps: WaitToolDeps, signal?: AbortSignal): Promise<Entry[]> {
	const filter = { from: args.from, threadId: args.threadId, after: args.after };
	const held = dispatcher.takeHeld(filter);
	return consumed(held.length > 0 ? held : await dispatcher.waitNext(filter, ms, signal), deps);
}

/**
 * The server's answer without the entries the session's own stream delivered; the rest count as consumed. When the
 * stream had delivered every entry of the answer, it waits on after the newest one until the deadline: in the
 * dispatcher once the session's stream is open, else at the server again.
 */
async function forwarded(args: WaitArgs, seconds: number, deps: WaitToolDeps, signal?: AbortSignal): Promise<unknown> {
	const deadline = Date.now() + seconds * 1000;
	for (let call = args; ; ) {
		const result = await deps.forward(call, waitSeconds(call) * 1000 + TRANSPORT_MARGIN_MS, signal);
		if (!Array.isArray(result) || !result.every(isEntry)) return result;
		const entries = deps.claim(result);
		const left = deadline - Date.now();
		if (entries.length > 0 || result.length === 0 || left <= 0) return consumed(entries, deps);
		call = { ...args, after: Math.max(call.after ?? 0, ...result.map(entry => entry.id)), timeoutSeconds: Math.ceil(left / 1000) };
		const dispatcher = deps.dispatcher();
		if (dispatcher) return local(dispatcher, call, left, deps, signal);
	}
}

async function wait(args: WaitArgs, deps: WaitToolDeps, signal?: AbortSignal): Promise<unknown> {
	const seconds = waitSeconds(args);
	const dispatcher = deps.dispatcher();
	return dispatcher ? local(dispatcher, args, seconds * 1000, deps, signal) : forwarded(args, seconds, deps, signal);
}

/** The local `bus_wait` of a main session, built on the server tool `tool` from `GET /v1/tools`. */
export function waitToolDefinition(pi: ExtensionAPI, tool: RemoteTool, deps: WaitToolDeps): Parameters<ExtensionAPI["registerTool"]>[0] {
	return {
		name: WAIT_TOOL,
		label: WAIT_TOOL,
		description: tool.description,
		parameters: toZod(tool.inputSchema, pi.zod),
		...presentationOf(WAIT_TOOL),
		execute: (_toolCallId: string, params: WaitArgs, signal: AbortSignal | undefined): Promise<ToolResult> =>
			toolResult(() => wait(params, deps, signal)),
	};
}
