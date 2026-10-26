/** The warning for an inbox stream refused with 409: another session already streams this agent's inbox. */
import type { BusClient } from "../bus/client.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";

/** The agent's name from `bus_whoami`, or undefined when the call fails or returns none. */
async function whoamiName(client: BusClient, active: ActiveCredentials): Promise<string | undefined> {
	try {
		const me = (await client.callTool(active.url, active.token, "bus_whoami", {})) as { name?: unknown } | null;
		return typeof me?.name === "string" && me.name ? me.name : undefined;
	} catch {
		// The name only decorates the warning; the warning itself must still be shown.
		return undefined;
	}
}

export async function conflictWarning(client: BusClient, active: ActiveCredentials): Promise<string> {
	const name = await whoamiName(client, active);
	return `another session is already connected as ${name ?? "this agent"}; inbox not delivered here`;
}
