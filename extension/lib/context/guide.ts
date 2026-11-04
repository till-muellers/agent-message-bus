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
	return {
		text: () => text,
		async load() {
			try {
				text = await fetchGuide();
				warned = false;
			} catch (error) {
				text = undefined;
				if (!warned) warn(`guide not loaded: ${errorMessage(error)}`);
				warned = true;
			}
		},
	};
}
