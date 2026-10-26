/** Mirrors the bus server's tool registry into the omp session. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { BusClient, RemoteTool } from "../bus/client.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";
import { errorMessage, NOT_LOGGED_IN } from "../bus/errors.ts";
import { toZod } from "../bus/schema.ts";
import { toolResult } from "./tool-result.ts";
import { attribute, ATTRIBUTED_TOOLS } from "./attribution.ts";
import { selectTools } from "./tool-selection.ts";

/** Per-session registration: tools are fetched and registered once; logout does not reset it. */
export interface RegistrationState {
	registered: boolean;
	/** Names of the tools registered so far, switched on or off by `restart()` to match the resolved credentials. */
	names: string[];
}

export interface RemoteToolsOptions {
	client: BusClient;
	/** The credentials active at call time; undefined after logout. */
	getActive: () => ActiveCredentials | undefined;
	state: RegistrationState;
	/** Reports a tool that could not be registered; the other tools still register. */
	warn: (message: string) => void;
	/** The kind of the session: a subagent gets the allow-list of `tool-selection.ts`. */
	kind: "main" | "sub";
	/** The id of the subagent that writes through this session; undefined in a main session. */
	agentId?: string;
}

function toolDefinition(pi: ExtensionAPI, tool: RemoteTool, options: RemoteToolsOptions): Parameters<ExtensionAPI["registerTool"]>[0] {
	const { client, getActive, kind, agentId } = options;
	return {
		name: tool.name,
		label: tool.name,
		description: tool.description,
		parameters: toZod(tool.inputSchema, pi.zod),
		execute: (_toolCallId: string, params: Record<string, unknown>) =>
			toolResult(async () => {
				const current = getActive();
				if (!current) throw new Error(NOT_LOGGED_IN);
				// A subagent says which subagent wrote the post.
				const attributed =
					kind === "sub" && agentId && ATTRIBUTED_TOOLS.includes(tool.name) ? attribute(params, agentId) : params;
				return client.callTool(current.url, current.token, tool.name, attributed);
			}),
	};
}

/** Registers the tools this session gets once per session. Returns the count registered by this call. */
export async function registerRemoteTools(pi: ExtensionAPI, options: RemoteToolsOptions): Promise<number> {
	const { client, getActive, state, warn, kind } = options;
	if (state.registered) return 0;
	const active = getActive();
	if (!active) return 0;
	const tools = await client.listTools(active.url, active.token);
	state.registered = true;
	for (const tool of selectTools(tools, kind)) {
		try {
			pi.registerTool(toolDefinition(pi, tool, options));
			state.names.push(tool.name);
		} catch (error) {
			warn(`tool ${tool.name} not registered: ${errorMessage(error)}`);
		}
	}
	return state.names.length;
}
