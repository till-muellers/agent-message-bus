/**
 * The one owner of the credentials this session acts with (client.md §2.1).
 *
 * The promoted leader token, the env credentials read at load and the member
 * token accepted by `/bus accept` live here, behind named operations: the
 * callers say what happened (`promote`, `loggedIn`, `inviteAccepted`,
 * `loggedOut`, `ended`) instead of reaching into the state. The precedence is
 * promote > env > accepted > the stored agent token.
 */
import type { ActiveCredentials, StoredCredentials } from "./stored-credentials.ts";
import { trimUrl } from "./stored-credentials.ts";

/** Where the active credentials came from. */
export type CredentialSource = "promote" | "env" | "accepted" | "stored-agent";

/** The credentials the session acts with, and where they came from. */
export interface ResolvedCredentials extends ActiveCredentials {
	source: CredentialSource;
}

/** The credential state of one session, behind named operations. */
export interface SessionCredentials {
	/** `/bus promote`: a leader token for this session only, never stored. */
	promote(creds: ActiveCredentials): void;
	/** `/bus login`: a new stored login replaces the promoted token. */
	loggedIn(): void;
	/** An accepted invite replaces the promoted and env credentials. */
	inviteAccepted(): void;
	/** `/bus logout`: drops promote and env. */
	loggedOut(): void;
	/** Session end: drops promote. */
	ended(): void;
	/** Only `session-identity.ts` calls this. */
	setAccepted(creds: ActiveCredentials | undefined): void;
	/** Precedence: promote > env > accepted > the stored agent token. */
	resolve(stored: StoredCredentials | undefined): ResolvedCredentials | undefined;
}

/** The credential state of one session; `env` holds the credentials read at load. */
export function createSessionCredentials(env: ActiveCredentials | undefined): SessionCredentials {
	let promoted: ActiveCredentials | undefined;
	let accepted: ActiveCredentials | undefined;
	return {
		promote(creds) {
			promoted = creds;
		},
		loggedIn() {
			promoted = undefined;
		},
		inviteAccepted() {
			promoted = undefined;
			env = undefined;
		},
		loggedOut() {
			promoted = undefined;
			env = undefined;
		},
		ended() {
			promoted = undefined;
		},
		setAccepted(creds) {
			accepted = creds;
		},
		resolve(stored) {
			if (promoted) return { ...promoted, source: "promote" };
			if (env) return { ...env, source: "env" };
			if (accepted) return { ...accepted, source: "accepted" };
			if (stored?.token) return { url: trimUrl(stored.url), token: stored.token, source: "stored-agent" };
			return undefined;
		},
	};
}
