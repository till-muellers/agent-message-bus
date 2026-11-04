/** Starts and stops the session's inbox loop (`inbox-loop.ts`), warning on errors and on another session holding the stream. */
import type { BusClient } from "../bus/client.ts";
import { conflictWarning } from "../connection/conflict.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";
import { errorMessage } from "../bus/errors.ts";
import { type InboxLoopOptions, runInboxLoop } from "./inbox-loop.ts";

export interface InboxRunnerOptions {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
	dispatcher: InboxLoopOptions["dispatcher"];
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
}

export interface InboxRunner {
	start(): void;
	stop(): void;
	/** Whether this session holds its inbox stream right now. */
	isOpen(): boolean;
}

export function createInboxRunner({ client, getActive, dispatcher, warn, sleep }: InboxRunnerOptions): InboxRunner {
	let loop: AbortController | undefined;
	let open = false;
	async function onConflict(): Promise<void> {
		const current = getActive();
		if (current) warn(await conflictWarning(client, current));
	}
	return {
		start() {
			const current = new AbortController();
			loop = current;
			const onError = (error: Error) => warn(error.message);
			// A stopped loop may still report its stream's end after the next loop opened its own.
			const onStream = (isOpen: boolean) => {
				if (loop === current) open = isOpen;
			};
			void runInboxLoop({ client, getActive, signal: current.signal, sleep, dispatcher, onError, onConflict, onStream }).catch(error =>
				warn(errorMessage(error)),
			);
		},
		stop() {
			loop?.abort();
			loop = undefined;
			open = false;
		},
		isOpen: () => open,
	};
}
