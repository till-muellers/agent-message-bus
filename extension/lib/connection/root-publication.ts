/**
 * A main session's entry in the root-identity registry (`session/root-identities.ts`, client.md §2.3): the
 * connection publishes the credentials it acts with under the session's current root key, one key at a time.
 */
import type { ActiveCredentials } from "../bus/credentials.ts";
import type { RootIdentities } from "../session/root-identities.ts";

export interface RootPublication {
	/** Publishes `creds` under `rootKey` (withdraws it without credentials) and withdraws the key published before. */
	set(rootKey: string | undefined, creds: ActiveCredentials | undefined): void;
	/** Withdraws what was published. */
	clear(): void;
}

export function createRootPublication(roots: Pick<RootIdentities, "publish" | "withdraw">): RootPublication {
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
