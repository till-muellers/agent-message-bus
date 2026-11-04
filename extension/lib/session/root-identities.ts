/**
 * The bus identity of each root session, for its subagents (client.md §2.3, S15).
 *
 * omp loads the extension module once per process and binds a fresh extension instance per
 * subagent, so the registry is created in the closure of `createBusExtension` and shared by a
 * main session and its subagents. The main session publishes the credentials it acts with under
 * its root key; a subagent reads them from here at every tool call, never from the store or the
 * env, so it acts as its main session's identity or not at all.
 */
import type { ActiveCredentials } from "./stored-credentials.ts";

export interface RootIdentities {
	/** Main session: publish the credentials it currently acts with (after every restart). */
	publish(rootKey: string, creds: ActiveCredentials): void;
	/** Main session: logout, shutdown. */
	withdraw(rootKey: string): void;
	/** Subagent session: read at every tool call; never the store, never the env. A session without a key has none. */
	lookup(rootKey: string | undefined): ActiveCredentials | undefined;
}

export function createRootIdentities(): RootIdentities {
	const byKey = new Map<string, ActiveCredentials>();
	return {
		publish: (rootKey, creds) => void byKey.set(rootKey, creds),
		withdraw: rootKey => void byKey.delete(rootKey),
		lookup: rootKey => (rootKey === undefined ? undefined : byKey.get(rootKey)),
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
