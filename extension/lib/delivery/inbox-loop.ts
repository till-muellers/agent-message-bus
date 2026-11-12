/** Inbox stream consumer: the reconnecting deliver+ack loop. */
import { BusError, type BusClient } from "../bus/client.ts";
import type { Entry } from "../bus/entry.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import { parseSse } from "../bus/sse.ts";

/** Where the loop puts the entries of the stream: the inbox ledger of the connection start (`inbox-ledger.ts`). */
export interface EntrySink {
	/** The stream delivered `entry`; new ids arrive ascending, a reconnect replays the unacked ones. */
	deliver(entry: Entry): void;
}

export interface InboxLoopOptions {
	client: BusClient;
	getActive: () => ActiveCredentials | undefined;
	signal: AbortSignal;
	/** Receives every entry of the stream; it hands it over to the session. */
	sink: EntrySink;
	/** Completes an entry before the sink gets it (`sender-reach.ts`); the next frame waits for it, so the order holds. */
	prepare?: (entry: Entry) => Promise<Entry>;
	onError: (error: Error) => void;
	/** Another session holds this agent's stream (409): called once per streak of refusals; the loop keeps retrying. */
	onConflict: (error: BusError) => void | Promise<void>;
	/** The stream opened (true) or ended (false); a refused open reports nothing. */
	onStream: (open: boolean) => void;
	/** The loop waits to open the stream again; not reported while another session holds it (`onConflict`). */
	onRetry?: () => void;
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

/** Puts every entry of an open stream into the sink; the ack waits for its consumption. */
async function consume(options: InboxLoopOptions, state: LoopState, body: ReadableStream<Uint8Array>): Promise<void> {
	for await (const frame of parseSse(body)) {
		if (frame.event !== "inbox") continue;
		const parsed = JSON.parse(frame.data) as Entry;
		const entry = options.prepare ? await options.prepare(parsed) : parsed;
		// A stopped loop hands nothing more to its sink.
		if (options.signal.aborted) return;
		options.sink.deliver(entry);
		state.delay = INITIAL_DELAY_MS;
	}
}

/** Consumes an open stream and reports it open until it ends, however it ends. */
async function consumeOpen(options: InboxLoopOptions, state: LoopState, body: ReadableStream<Uint8Array>): Promise<void> {
	options.onStream(true);
	try {
		await consume(options, state, body);
	} finally {
		options.onStream(false);
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
			await consumeOpen(options, state, response.body);
		} catch (error) {
			if (signal.aborted || (await report(error, options, state))) return;
		}
		if (signal.aborted) return;
		if (!state.conflicted) options.onRetry?.();
		await sleep(state.delay, signal);
		state.delay = Math.min(state.delay * 2, MAX_DELAY_MS);
	}
}
