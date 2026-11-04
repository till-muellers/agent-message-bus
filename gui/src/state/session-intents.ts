import { ApiError, errorMessage } from "../api/api-error.ts";
import type { SessionApi } from "../api/session.ts";
import type { SessionInfo } from "../api/types.ts";
import type { Action } from "./actions.ts";

export interface SessionIntents {
	signIn(token: string): Promise<void>;
	restore(): Promise<void>;
	signOut(): Promise<void>;
}

/** Runs the session calls and dispatches their outcome; never rejects. */
export function createSessionIntents(api: SessionApi, dispatch: (action: Action) => void): SessionIntents {
	const signedIn = (info: SessionInfo): void => dispatch({ type: "session/restored", ...info });
	const signedOut = (error?: string): void => dispatch({ type: "session/signed-out", error });
	return {
		signIn: (token) => api.signIn(token).then(signedIn, (err: unknown) => signedOut(errorMessage(err))),
		restore: () => api.restore().then(signedIn, (err: unknown) => signedOut(unlessSignedOut(err))),
		signOut: () =>
			api.signOut().then(
				() => signedOut(),
				(err: unknown) => signedOut(unlessSignedOut(err, "sign-out not confirmed by the server: ")),
			),
	};
}

/** No message when the server says there is no session (401): that is just the login. */
function unlessSignedOut(err: unknown, prefix = ""): string | undefined {
	return err instanceof ApiError && err.status === 401 ? undefined : prefix + errorMessage(err);
}
