/** Starts and stops the session's inbox loop (`lib/inbox.ts`), warning on errors and on another session holding the stream. */
import type { BusClient } from "./bus/client.ts";
import { conflictWarning } from "./conflict.ts";
import type { ActiveCredentials } from "./credentials.ts";
import { errorMessage } from "./bus/errors.ts";
import { type InboxLoopOptions, runInboxLoop } from "./inbox.ts";

export interface InboxRunnerOptions {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
	deliver: InboxLoopOptions["deliver"];
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
}

export interface InboxRunner {
	start(): void;
	stop(): void;
}

export function createInboxRunner({ client, getActive, deliver, warn, sleep }: InboxRunnerOptions): InboxRunner {
	let loop: AbortController | undefined;
	async function onConflict(): Promise<void> {
		const current = getActive();
		if (current) warn(await conflictWarning(client, current));
	}
	return {
		start() {
			loop = new AbortController();
			const onError = (error: Error) => warn(error.message);
			void runInboxLoop({ client, getActive, signal: loop.signal, sleep, deliver, onError, onConflict }).catch(error => warn(errorMessage(error)));
		},
		stop() {
			loop?.abort();
			loop = undefined;
		},
	};
}
