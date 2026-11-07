/**
 * `bus_wait` in omp: a local tool with the server tool's name, description and schema (`docs/design/arch/client.md` §3.4).
 * With the session's own inbox stream it returns the entries the inbox ledger still holds at once, else it waits in the
 * ledger, so the entries it returns never reach omp a second time; the ledger counts them as consumed. Without one it
 * forwards the call to the server's `bus_wait`, which acks nothing. What a forwarded call returns is claimed from the
 * ledger: an entry the session's stream delivered meanwhile is left out, and one it delivers later is skipped there.
 * When that leaves nothing, the call waits on after the newest entry for the time left (in the ledger once the session's
 * stream is open, else at the server), so [] still means timeout. The tool is `interruptible`: an entry queued for the
 * model ends the wait with [] and reaches the model itself.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { BusError, type RemoteTool } from "../bus/client.ts";
import type { Entry } from "../bus/entry.ts";
import { toZod } from "../bus/schema.ts";
import type { WaitFilter } from "../delivery/waiters.ts";
import { presentationOf } from "./tool-policy.ts";
import { type ToolResult, toolResult } from "./tool-result.ts";

export const WAIT_TOOL = "bus_wait";

/** The arguments of `bus_wait` (contracts.md §2.5). */
export interface WaitArgs extends WaitFilter {
	timeoutSeconds?: number;
}

/** What `bus_wait` uses of the inbox ledger of the session's open stream; the entries it returns count as consumed there. */
export interface WaitSource {
	takeHeld(filter: WaitFilter): Entry[];
	awaitEntry(filter: WaitFilter, ms: number, signal?: AbortSignal): Promise<Entry[]>;
}

/** What a forwarded `bus_wait` uses of the inbox ledger of the current connection start. */
export interface ClaimView {
	/** The entries the session's stream has not delivered, claimed and consumed. */
	claimFromServer(entries: readonly Entry[]): Entry[];
}

export interface WaitToolDeps {
	/** The ledger of the session's open inbox stream; undefined while the session holds none. */
	source(): WaitSource | undefined;
	/** Claims what a forwarded call returned. */
	claims: ClaimView;
	/** Calls the server's `bus_wait` with a transport timeout. */
	forward(args: WaitArgs, timeoutMs: number, signal?: AbortSignal): Promise<unknown>;
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

/** In the ledger: the held entries matching `args` at once, else the next one within `ms`. */
async function local(source: WaitSource, args: WaitArgs, ms: number, signal?: AbortSignal): Promise<Entry[]> {
	const filter = { from: args.from, threadId: args.threadId, after: args.after };
	const held = source.takeHeld(filter);
	return held.length > 0 ? held : source.awaitEntry(filter, ms, signal);
}

/**
 * The server's answer without the entries the session's own stream delivered; the rest are claimed. When the stream
 * had delivered every entry of the answer, it waits on after the newest one until the deadline: in the ledger once the
 * session's stream is open, else at the server again.
 */
async function forwarded(args: WaitArgs, seconds: number, deps: WaitToolDeps, signal?: AbortSignal): Promise<unknown> {
	const deadline = Date.now() + seconds * 1000;
	for (let call = args; ; ) {
		const result = await deps.forward(call, waitSeconds(call) * 1000 + TRANSPORT_MARGIN_MS, signal);
		if (!Array.isArray(result) || !result.every(isEntry)) return result;
		const entries = deps.claims.claimFromServer(result);
		const left = deadline - Date.now();
		if (entries.length > 0 || result.length === 0 || left <= 0) return entries;
		call = { ...args, after: Math.max(call.after ?? 0, ...result.map(entry => entry.id)), timeoutSeconds: Math.ceil(left / 1000) };
		const source = deps.source();
		if (source) return local(source, call, left, signal);
	}
}

async function wait(args: WaitArgs, deps: WaitToolDeps, signal?: AbortSignal): Promise<unknown> {
	const seconds = waitSeconds(args);
	const source = deps.source();
	return source ? local(source, args, seconds * 1000, signal) : forwarded(args, seconds, deps, signal);
}

/**
 * omp's agent loop aborts the signal of a running `interruptible` tool with the interrupt reason once an aside, steer or
 * interrupt is queued; `ToolDefinition` does not declare the field, `applyToolProxy` copies it onto the running tool.
 */
export type WaitToolDefinition = Parameters<ExtensionAPI["registerTool"]>[0] & { interruptible: true };

/** The local `bus_wait` of a main session, built on the server tool `tool` from `GET /v1/tools`, whose name it keeps. */
export function waitToolDefinition(pi: ExtensionAPI, tool: RemoteTool, deps: WaitToolDeps): WaitToolDefinition {
	return {
		name: tool.name,
		label: tool.name,
		description: tool.description,
		parameters: toZod(tool.inputSchema, pi.zod),
		...presentationOf(tool.name),
		interruptible: true,
		execute: (_toolCallId: string, params: WaitArgs, signal: AbortSignal | undefined): Promise<ToolResult> =>
			toolResult(() => wait(params, deps, signal)),
	};
}
