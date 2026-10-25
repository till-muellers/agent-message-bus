/** Mirrors the bus server's tool registry into the omp session. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { BusError, type BusClient, type RemoteTool } from "./client.ts";
import type { ActiveCredentials } from "./credentials.ts";
import { toZod } from "./schema.ts";

/** Per-session flag: tools are fetched and registered once; logout does not reset it. */
export interface RegistrationState {
	registered: boolean;
}

export interface RemoteToolsOptions {
	client: BusClient;
	/** The credentials active at call time; undefined after logout. */
	getActive: () => ActiveCredentials | undefined;
	state: RegistrationState;
	/** Reports a tool that could not be registered; the other tools still register. */
	warn: (message: string) => void;
}

function toolDefinition(pi: ExtensionAPI, tool: RemoteTool, { client, getActive }: RemoteToolsOptions): Parameters<ExtensionAPI["registerTool"]>[0] {
	return {
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
	};
}

/** Registers every server tool once per session. Returns the count registered by this call. */
export async function registerRemoteTools(pi: ExtensionAPI, options: RemoteToolsOptions): Promise<number> {
	const { client, getActive, state, warn } = options;
	if (state.registered) return 0;
	const active = getActive();
	if (!active) return 0;
	const tools = await client.listTools(active.url, active.token);
	state.registered = true;
	let count = 0;
	for (const tool of tools) {
		try {
			pi.registerTool(toolDefinition(pi, tool, options));
			count++;
		} catch (error) {
			warn(`tool ${tool.name} not registered: ${error instanceof Error ? error.message : String(error)}`);
		}
	}
	return count;
}
