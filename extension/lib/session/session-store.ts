/**
 * Per-session member credentials from `/bus accept`: `{url, token}` in
 * `<dir>/sessions/<key>.json` (0600), keyed by the omp session file's name so
 * `omp --resume` / `-c` (which reopen the same file) keep the identity.
 * Subagent sessions live in `<parent>/<id>.jsonl` and use their interactive
 * root's key, so they act as the agent their top-level session accepted.
 */
import { existsSync } from "node:fs";
import { basename, dirname, join } from "node:path";
import type { ActiveCredentials } from "./stored-credentials.ts";
import { createJsonFileStore, type JsonFileStore } from "../shared/credential-store.ts";

export type SessionStore = JsonFileStore<ActiveCredentials>;

/** How far a subagent session walks up to its root; mirrors omp's `resolveBreadcrumbToInteractiveRoot`. */
const MAX_NESTING = 8;

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

/** The top-level session file: walks up while the containing dir is a session's artifacts dir (`<dir>.jsonl` exists). */
function interactiveRoot(sessionFile: string): string {
	let current = sessionFile;
	for (let depth = 0; depth < MAX_NESTING; depth++) {
		const parent = `${dirname(current)}.jsonl`;
		if (!existsSync(parent)) break;
		current = parent;
	}
	return current;
}

/** The store key of a session: its file name without `.jsonl`; undefined for a session without a file. */
export function sessionKey(sessionFile: string | undefined, sub: boolean): string | undefined {
	if (!sessionFile) return undefined;
	return basename(sub ? interactiveRoot(sessionFile) : sessionFile, ".jsonl");
}
