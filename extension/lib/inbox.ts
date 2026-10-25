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

/** Acks up to `id`. A failure is only reported: reconnecting would redeliver the entry, and the next ack's higher `upTo` covers it. */
async function ack(client: BusClient, active: ActiveCredentials, id: number, onError: (error: Error) => void): Promise<void> {
	try {
		await client.callTool(active.url, active.token, "bus_inbox_ack", { upTo: id });
	} catch (error) {
		onError(error instanceof Error ? error : new Error(String(error)));
	}
}

/** Streams the inbox until `signal` aborts or the token is rejected (401/403). */
export async function runInboxLoop(options: InboxLoopOptions): Promise<void> {
	const { client, getActive, signal, deliver, onError, sleep = abortableSleep } = options;
	let delay = INITIAL_DELAY_MS;
	while (!signal.aborted) {
		const active = getActive();
		if (!active) return;
		try {
			const response = await client.openStream(active.url, active.token, signal);
			if (!response.body) throw new Error("inbox stream has no body");
			for await (const frame of parseSse(response.body)) {
				if (frame.event !== "inbox") continue;
				const entry = JSON.parse(frame.data) as Entry;
				await deliver(entry);
				await ack(client, active, entry.id, onError);
				delay = INITIAL_DELAY_MS;
			}
		} catch (error) {
			if (signal.aborted) return;
			onError(error instanceof Error ? error : new Error(String(error)));
			if (isFatal(error)) return;
		}
		if (signal.aborted) return;
		await sleep(delay, signal);
		delay = Math.min(delay * 2, MAX_DELAY_MS);
	}
}
