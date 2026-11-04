/**
 * The server's agent guide (`GET /v1/guide`, `docs/design/arch/contracts.md` §7.1), loaded once per connection for
 * the main session's context block. A server without it (404) or a failed load leaves no guide; failing loads
 * in a row are logged once.
 */
import { errorMessage } from "../bus/errors.ts";

export interface Guide {
	/** The guide of the last load; undefined before it or when it failed. */
	text(): string | undefined;
	load(): Promise<void>;
}

export function createGuide(fetchGuide: () => Promise<string>, warn: (message: string) => void): Guide {
	let text: string | undefined;
	let warned = false;
	// A load that a later one overtook (a restart against another server) must not write its result.
	let generation = 0;
	return {
		text: () => text,
		async load() {
			const mine = ++generation;
			try {
				const loaded = await fetchGuide();
				if (mine !== generation) return;
				text = loaded;
				warned = false;
			} catch (error) {
				if (mine !== generation) return;
				text = undefined;
				if (!warned) warn(`guide not loaded: ${errorMessage(error)}`);
				warned = true;
			}
		},
	};
}
