/**
 * Which bus tools a session registers: a subagent gets an allow-list, the main session gets
 * everything else (`docs/design/arch/client.md` §6.1).
 *
 * The withholding happens at registration, not at activation: omp enables every tool an extension
 * registers unless it is `defaultInactive` or hidden.
 */
import type { RemoteTool } from "../bus/client.ts";

/** The tools a subagent gets: an allow-list, so a tool the server adds later stays main-only. */
export const SUBAGENT_TOOLS: readonly string[] = [
	"bus_list_blackboards",
	"bus_list_threads",
	"bus_read_thread",
	"bus_create_thread",
	"bus_add_post",
];

/** The server tools that omp provides itself in a main session. */
export const SHADOWED_TOOLS: readonly string[] = ["bus_wait"];

/** The server tools the extension owns: the acknowledgement follows the consumption. */
export const WITHHELD_TOOLS: readonly string[] = ["bus_inbox_ack"];

/** The tools a session of this kind registers. */
export function selectTools(remote: readonly RemoteTool[], kind: "main" | "sub"): RemoteTool[] {
	if (kind === "sub") return remote.filter(tool => SUBAGENT_TOOLS.includes(tool.name));
	return remote.filter(tool => !SHADOWED_TOOLS.includes(tool.name) && !WITHHELD_TOOLS.includes(tool.name));
}
