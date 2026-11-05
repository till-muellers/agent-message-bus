/**
 * Starts and stops the session's inbox loop (`inbox-loop.ts`), warning on errors and on another session holding the stream.
 * Each start streams into the sink it is given, the inbox ledger of that connection start; stopping stops that sink.
 */
import type { BusClient } from "../bus/client.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import { conflictWarning } from "./conflict.ts";
import { errorMessage } from "../bus/errors.ts";
import type { ConnectionState } from "./bus-events.ts";
import { type EntrySink, type InboxLoopOptions, runInboxLoop } from "./inbox-loop.ts";

/** What one start streams into: the entries, and the end of the start's stream. */
export interface StartSink extends EntrySink {
	/** The start's stream stopped: drop what is held, end what waits. */
	stop(): void;
}

export interface InboxRunnerOptions {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
	/** The stream opened, another session holds it, or the loop waits to retry. */
	onState?: (state: Exclude<ConnectionState, "logged-out">) => void;
}

export interface InboxRunner {
	/** Streams the inbox into `sink` until `stop()`. */
	start(sink: StartSink): void;
	stop(): void;
	/** Whether this session holds its inbox stream right now. */
	isOpen(): boolean;
}

export function createInboxRunner({ client, getActive, warn, sleep, onState }: InboxRunnerOptions): InboxRunner {
	let loop: AbortController | undefined;
	let streamed: StartSink | undefined;
	let open = false;
	async function onConflict(): Promise<void> {
		onState?.("conflict");
		const current = getActive();
		if (current) warn(await conflictWarning(client, current));
	}
	return {
		start(sink) {
			const current = new AbortController();
			loop = current;
			streamed = sink;
			const onError = (error: Error) => warn(error.message);
			// A stopped loop may still report its stream's end after the next loop opened its own.
			const onStream = (isOpen: boolean) => {
				if (loop !== current) return;
				open = isOpen;
				if (isOpen) onState?.("connected");
			};
			const onRetry = () => onState?.("retrying");
			void runInboxLoop({ client, getActive, signal: current.signal, sleep, sink, onError, onConflict, onStream, onRetry }).catch(error =>
				warn(errorMessage(error)),
			);
		},
		stop() {
			loop?.abort();
			loop = undefined;
			open = false;
			// Held entries were never handed over: the next start's replay delivers them.
			streamed?.stop();
			streamed = undefined;
		},
		isOpen: () => open,
	};
}
