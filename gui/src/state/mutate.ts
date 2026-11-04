import { ApiError, errorMessage } from "../api/api-error.ts";
import type { Action } from "./actions.ts";

export interface MutationDeps {
	dispatch(action: Action): void;
	/** Reloads the current page (the poller's refreshNow). */
	refresh(): void;
}

/**
 * Awaits a mutation and dispatches its outcome, then refreshes the page; never rejects.
 * A 401 signs out with the reason; any other failure shows the server's message.
 */
export async function mutate<T>(deps: MutationDeps, call: Promise<T>, done: (result: T) => Action): Promise<void> {
	let result: T;
	try {
		result = await call;
	} catch (err) {
		deps.dispatch(refused(err));
		return;
	}
	deps.dispatch(done(result));
	deps.refresh();
}

function refused(err: unknown): Action {
	if (err instanceof ApiError && err.status === 401) return { type: "session/signed-out", error: err.message };
	return { type: "notice/shown", message: errorMessage(err), alert: true };
}
