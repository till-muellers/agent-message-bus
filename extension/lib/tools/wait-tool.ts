/**
 * `bus_wait` in omp: a local tool with the server tool's name, description and schema (`docs/design/arch/client.md` §3.4).
 * With the session's own inbox stream it waits in the dispatcher, so the entry it returns never reaches omp a second
 * time and counts as consumed; without one it forwards the call to the server's `bus_wait`, which acks nothing.
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

export interface WaitToolDeps {
	/** The dispatcher of the session's open inbox stream; undefined while the session holds none. */
	dispatcher(): Pick<Dispatcher, "waitNext"> | undefined;
	/** Calls the server's `bus_wait` with a transport timeout. */
	forward(args: WaitArgs, timeoutMs: number, signal?: AbortSignal): Promise<unknown>;
	/** The ids the tool returned from the stream: consumed. */
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

async function wait(args: WaitArgs, deps: WaitToolDeps, signal?: AbortSignal): Promise<unknown> {
	const seconds = waitSeconds(args);
	const dispatcher = deps.dispatcher();
	if (!dispatcher) return deps.forward(args, seconds * 1000 + TRANSPORT_MARGIN_MS, signal);
	const { from, threadId, after } = args;
	const entries: Entry[] = await dispatcher.waitNext({ from, threadId, after }, seconds * 1000, signal);
	if (entries.length > 0) deps.consumed(entries.map(entry => entry.id));
	return entries;
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
