/**
 * Per-session member credentials from `/bus accept`: `{url, token}` in
 * `<dir>/sessions/<key>.json` (0600), keyed by the omp session file's name so
 * `omp --resume` / `-c` (which reopen the same file) keep the identity.
 * Subagents never read it: they act with the identity their main session
 * publishes (`root-identities.ts`).
 */
import { basename, join } from "node:path";
import type { ActiveCredentials } from "./stored-credentials.ts";
import { createJsonFileStore, type JsonFileStore } from "../shared/credential-store.ts";

export type SessionStore = JsonFileStore<ActiveCredentials>;

function parseActive(json: unknown): ActiveCredentials {
	const parsed = json as Partial<Record<keyof ActiveCredentials, unknown>> | null;
	const { url, token } = parsed ?? {};
	if (typeof url !== "string" || !url || typeof token !== "string" || !token) throw new Error("no credentials");
	return { url, token };
}

export function createSessionStore(dir: string, key: string): SessionStore {
	return createJsonFileStore({
		dir: join(dir, "sessions"),
		file: `${key}.json`,
		parse: parseActive,
		unreadable: path => new Error(`Session credential file ${path} is unreadable. Run /bus accept again.`),
	});
}

/** The store key of a session: its file name without `.jsonl`; undefined for a session without a file. */
export function sessionKey(sessionFile: string | undefined): string | undefined {
	return sessionFile ? basename(sessionFile, ".jsonl") : undefined;
}
