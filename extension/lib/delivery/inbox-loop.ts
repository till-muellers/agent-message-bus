/** Inbox stream consumer: the reconnecting deliver+ack loop. */
import { BusError, type BusClient } from "../bus/client.ts";
import type { Entry } from "../bus/entry.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";
import { parseSse } from "../bus/sse.ts";

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

/** Delivers and acks each entry of an open stream. */
async function consume(options: InboxLoopOptions, active: ActiveCredentials, state: LoopState, body: ReadableStream<Uint8Array>): Promise<void> {
	for await (const frame of parseSse(body)) {
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
			state.conflicted = false;
			await consume(options, active, state, response.body);
		} catch (error) {
			if (signal.aborted || (await report(error, options, state))) return;
		}
		if (signal.aborted) return;
		await sleep(state.delay, signal);
		state.delay = Math.min(state.delay * 2, MAX_DELAY_MS);
	}
}
