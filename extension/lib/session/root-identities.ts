/**
 * The bus identity of each root session, for its subagents (client.md §2.3, S15).
 *
 * omp loads the extension module once per process and binds a fresh extension instance per
 * subagent, so the registry is created in the closure of `createBusExtension` and shared by a
 * main session and its subagents. The main session publishes the credentials it acts with under
 * its root key; a subagent reads them from here at every tool call, never from the store or the
 * env, so it acts as its main session's identity or not at all.
 *
 * The root key is the main session's file without `.jsonl`: the artifacts dir its subagents'
 * files live in (`<root>/<id>.jsonl`, nested `<root>/<id>/<id2>.jsonl`). A subagent finds it by
 * walking up its own file's dirs, so the main session's file need not exist on disk yet (omp
 * writes it only after the first assistant message).
 */
import { dirname, resolve } from "node:path";
import type { ActiveCredentials } from "../bus/credentials.ts";

/** How far a subagent session walks up to its root; mirrors omp's `resolveBreadcrumbToInteractiveRoot`. */
const MAX_NESTING = 8;

export interface RootIdentities {
	/** Main session: publish the credentials it currently acts with (after every restart). */
	publish(rootKey: string, creds: ActiveCredentials): void;
	/** Main session: logout, shutdown. */
	withdraw(rootKey: string): void;
	/** Subagent session: the identity of the nearest root session that holds `sessionFile`; never the store, never the env. */
	lookup(sessionFile: string | undefined): ActiveCredentials | undefined;
}

/** The key a main session publishes under; undefined for a session without a file (`--no-session`). */
export function rootKeyOf(sessionFile: string | undefined): string | undefined {
	return sessionFile ? resolve(sessionFile).replace(/\.jsonl$/, "") : undefined;
}

export function createRootIdentities(): RootIdentities {
	const byKey = new Map<string, ActiveCredentials>();
	return {
		publish: (rootKey, creds) => void byKey.set(rootKey, creds),
		withdraw: rootKey => void byKey.delete(rootKey),
		lookup(sessionFile) {
			if (!sessionFile) return undefined;
			let dir = dirname(resolve(sessionFile));
			for (let depth = 0; depth < MAX_NESTING; depth++) {
				const found = byKey.get(dir);
				if (found) return found;
				dir = dirname(dir);
			}
			return undefined;
		},
	};
}

/** A main session's entry in the registry: one key at a time, the session's current one. */
export interface RootPublication {
	/** Publishes `creds` under `rootKey` (withdraws it without credentials) and withdraws the key published before. */
	set(rootKey: string | undefined, creds: ActiveCredentials | undefined): void;
	/** Withdraws what was published. */
	clear(): void;
}

export function createRootPublication(roots: RootIdentities): RootPublication {
	let published: string | undefined;
	const clear = () => {
		if (published !== undefined) roots.withdraw(published);
		published = undefined;
	};
	return {
		set(rootKey, creds) {
			clear();
			if (rootKey === undefined || !creds) return;
			roots.publish(rootKey, creds);
			published = rootKey;
		},
		clear,
	};
}
