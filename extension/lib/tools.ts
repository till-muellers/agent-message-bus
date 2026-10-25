/** Mirrors the bus server's tool registry into the omp session. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { BusError, type BusClient } from "./client.ts";
import type { ActiveCredentials } from "./credentials.ts";
import { toZod } from "./schema.ts";

/** Per-session flag: tools are fetched and registered once. */
export interface RegistrationState {
	registered: boolean;
}

/** Registers every server tool; `execute` uses the credentials active at call time. Returns the count registered now. */
export async function registerRemoteTools(
	pi: ExtensionAPI,
	client: BusClient,
	getActive: () => ActiveCredentials | undefined,
	state: RegistrationState,
): Promise<number> {
	if (state.registered) return 0;
	const active = getActive();
	if (!active) return 0;
	const tools = await client.listTools(active.url, active.token);
	state.registered = true;
	for (const tool of tools) {
		pi.registerTool({
			name: tool.name,
			label: tool.name,
			description: tool.description,
			parameters: toZod(tool.inputSchema, pi.zod),
			async execute(_toolCallId: string, params: Record<string, unknown>) {
				const current = getActive();
				if (!current) throw new Error("Not logged in to the agent bus. Run /bus login <url> <token>.");
				try {
					const result = await client.callTool(current.url, current.token, tool.name, params);
					return { content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }] };
				} catch (error) {
					if (error instanceof BusError) throw new Error(`${error.code}: ${error.message}`);
					throw error;
				}
			},
		});
	}
	return tools.length;
}
