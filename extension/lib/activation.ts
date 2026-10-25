/** Switches the session's `bus_*` tools on and off: logout deactivates them, login and `/bus server start` re-activate them. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";

type ToolHost = Pick<ExtensionAPI, "getActiveTools" | "setActiveTools">;

/** Removes every `bus_*` tool from the active set; other tools stay. */
export async function deactivateBusTools(pi: ToolHost): Promise<void> {
	await pi.setActiveTools(pi.getActiveTools().filter(name => !name.startsWith("bus_")));
}

/** Adds `names` (the registered bus tools) back to the active set. */
export async function activateBusTools(pi: ToolHost, names: readonly string[]): Promise<void> {
	const active = pi.getActiveTools();
	const missing = names.filter(name => !active.includes(name));
	if (missing.length) await pi.setActiveTools([...active, ...missing]);
}
