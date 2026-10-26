/**
 * The acker: it tells the server how far the session consumed the inbox. A failure is only warned:
 * the entries stay on the server and are replayed (`docs/design/arch/client.md` §3.3).
 */
import type { BusClient } from "../bus/client.ts";
import { errorMessage } from "../bus/errors.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";

export interface AckerDeps {
	client: BusClient;
	/** The credentials the session acts with; undefined when it has none. */
	getActive(): ActiveCredentials | undefined;
	warn(message: string): void;
}

/** The acker of one session. */
export function createAcker(deps: AckerDeps): (upTo: number) => Promise<void> {
	return async upTo => {
		const active = deps.getActive();
		if (!active) return;
		try {
			await deps.client.callTool(active.url, active.token, "bus_inbox_ack", { upTo });
		} catch (error) {
			deps.warn(errorMessage(error));
		}
	};
}
