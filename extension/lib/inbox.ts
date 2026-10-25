/** Inbox stream consumer: the wire entry type and the reconnecting deliver+ack loop. */
import { BusError, type BusClient } from "./client.ts";
import type { ActiveCredentials } from "./credentials.ts";
import { parseSse } from "./sse.ts";

export type EntryType = "direct_message" | "topic_message" | "topic_closed" | "thread_post" | "thread_removed" | "blackboard_removed";

export interface Entry {
	id: number;
	space: string;
	type: EntryType;
	from: string;
	critical: boolean;
	topic?: string;
	blackboard?: string;
	threadId?: number;
	threadTitle?: string;
	body?: string;
	createdAt: string;
}

export interface InboxLoopOptions {
	client: BusClient;
	getActive: () => ActiveCredentials | undefined;
	signal: AbortSignal;
	deliver: (entry: Entry) => Promise<void>;
	onError: (error: Error) => void;
	/** Another session holds this agent's stream (409): called once per streak of refusals; the loop keeps retrying. */
	onConflict: (error: BusError) => void | Promise<void>;
	/** Backoff wait; injectable for tests. */
	sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
}

const INITIAL_DELAY_MS = 1000;
const MAX_DELAY_MS = 30_000;

function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
	const { promise, resolve } = Promise.withResolvers<void>();
	const timer = setTimeout(done, ms);
	function done() {
		clearTimeout(timer);
		signal.removeEventListener("abort", done);
		resolve();
	}
	signal.addEventListener("abort", done, { once: true });
	return promise;
}

function isFatal(error: unknown): boolean {
	return error instanceof BusError && (error.code === "unauthenticated" || error.code === "forbidden");
}

/** Backoff delay and whether the current run of failed opens are conflicts already reported. */
interface LoopState {
	delay: number;
	conflicted: boolean;
}

/** Acks up to `id`. A failure is only reported: reconnecting would redeliver the entry, and the next ack's higher `upTo` covers it. */
async function ack(client: BusClient, active: ActiveCredentials, id: number, onError: (error: Error) => void): Promise<void> {
	try {
		await client.callTool(active.url, active.token, "bus_inbox_ack", { upTo: id });
	} catch (error) {
		onError(error instanceof Error ? error : new Error(String(error)));
	}
}

/** Delivers and acks each entry of an open stream; any frame ends a conflict streak. */
async function consume(options: InboxLoopOptions, active: ActiveCredentials, state: LoopState, body: ReadableStream<Uint8Array>): Promise<void> {
	for await (const frame of parseSse(body)) {
		state.conflicted = false;
		if (frame.event !== "inbox") continue;
		const entry = JSON.parse(frame.data) as Entry;
		await options.deliver(entry);
		await ack(options.client, active, entry.id, options.onError);
		state.delay = INITIAL_DELAY_MS;
	}
}

/** Reports a stream failure (a conflict only once per streak). Returns true when the token was rejected and the loop must stop. */
async function report(error: unknown, options: InboxLoopOptions, state: LoopState): Promise<boolean> {
	if (error instanceof BusError && error.code === "conflict") {
		if (!state.conflicted) await options.onConflict(error);
		state.conflicted = true;
		return false;
	}
	options.onError(error instanceof Error ? error : new Error(String(error)));
	return isFatal(error);
}

/** Streams the inbox until `signal` aborts or the token is rejected (401/403); other failures, conflicts included, retry with backoff. */
export async function runInboxLoop(options: InboxLoopOptions): Promise<void> {
	const { client, getActive, signal, sleep = abortableSleep } = options;
	const state: LoopState = { delay: INITIAL_DELAY_MS, conflicted: false };
	while (!signal.aborted) {
		const active = getActive();
		if (!active) return;
		try {
			const response = await client.openStream(active.url, active.token, signal);
			if (!response.body) throw new Error("inbox stream has no body");
			await consume(options, active, state, response.body);
		} catch (error) {
			if (signal.aborted || (await report(error, options, state))) return;
		}
		if (signal.aborted) return;
		await sleep(state.delay, signal);
		state.delay = Math.min(state.delay * 2, MAX_DELAY_MS);
	}
}
