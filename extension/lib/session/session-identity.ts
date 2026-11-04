/**
 * The member identity this omp session accepted with `/bus accept`: owns the session's key, its credential file
 * (`session-store.ts`) and the accepted token of the credential set, which nothing else writes.
 */
import type { SessionCredentials } from "./session-credentials.ts";
import type { ActiveCredentials } from "./stored-credentials.ts";
import { createSessionStore, type SessionStore, sessionKey } from "./session-store.ts";

/** The part of the omp context that identifies a session: its session file. */
export interface IdentityContext {
	sessionManager?: { getSessionFile(): string | undefined };
}

/**
 * How the session being bound relates to the one bound before, as omp's event says: the same session (start, a
 * command), another session (`/new`, `/resume`), or a branch or fork of `previousSessionFile`, which omp always moves
 * onto a new session file.
 */
export type SessionChange = { kind: "same" } | { kind: "new" } | { kind: "branch"; previousSessionFile: string | undefined };

export interface SessionIdentity {
	/** Keys the identity by `ctx`'s session file; `change` decides what happens to the accepted token. */
	bind(ctx: IdentityContext, change?: SessionChange): Promise<void>;
	/** Reads the bound session's file into `accepted`; a session without a file keeps the token held in memory. */
	load(): Promise<void>;
	/** Makes `creds` the accepted token and writes it to the session's file. Resolves false when there is no file. */
	save(creds: ActiveCredentials): Promise<boolean>;
	/** Forgets the accepted token and deletes the session's file. */
	clear(): Promise<void>;
}

/** `dir` holds `sessions/<key>.json`; `credentials` is the session's credential set whose accepted token this owns. */
export function createSessionIdentity(dir: string, credentials: SessionCredentials): SessionIdentity {
	let store: SessionStore | undefined;
	const storeOf = (file: string | undefined) => {
		const key = sessionKey(file);
		return key ? createSessionStore(dir, key) : undefined;
	};
	return {
		async bind(ctx, change = { kind: "same" }) {
			store = storeOf(ctx.sessionManager?.getSessionFile());
			if (change.kind === "new") credentials.setAccepted(undefined);
			if (change.kind !== "branch") return;
			const previous = storeOf(change.previousSessionFile);
			// A branch without a previous session file keeps the token it already holds.
			if (!previous) return;
			const carried = await previous.load();
			credentials.setAccepted(carried);
			if (carried) await store?.save(carried);
		},
		async load() {
			if (store) credentials.setAccepted(await store.load());
		},
		async save(creds) {
			await store?.save(creds);
			credentials.setAccepted(creds);
			return store !== undefined;
		},
		async clear() {
			credentials.setAccepted(undefined);
			await store?.clear();
		},
	};
}
