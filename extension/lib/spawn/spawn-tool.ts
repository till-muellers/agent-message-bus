/**
 * `bus_spawn_worker`: the agent-facing form of `/bus spawn`, a local tool next to the server's `bus_*` tools. It is
 * registered inactive and switched on with them once the session has credentials.
 */
import { resolve } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { type JsonSchema, toZod } from "../bus/schema.ts";
import type { SpawnRequest, SpawnResult } from "./spawn.ts";
import { toolResult } from "../tools/tool-result.ts";

export const SPAWN_TOOL = "bus_spawn_worker";

interface SpawnParams {
	space: string;
	name: string;
	task?: string;
	cwd?: string;
}

export type Spawn = (req: SpawnRequest, signal?: AbortSignal) => Promise<SpawnResult>;

const DESCRIPTION =
	"Leaders of the space only. Invites member <name> and starts it as a separate omp session in a new terminal tab (Orca, tmux, zellij, WezTerm, kitty, Windows Terminal; else a new window). The worker joins the space by itself and gets <task> as its first prompt. Returns {name, space, launcher}; the invite code is never exposed.";

const PARAMETERS: JsonSchema = {
	type: "object",
	properties: {
		space: { type: "string", description: "Space name." },
		name: { type: "string", description: "Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$." },
		task: { type: "string", description: "First prompt for the worker." },
		cwd: { type: "string", description: "Working directory, relative to this session's; defaults to this session's." },
	},
	required: ["space", "name"],
	additionalProperties: false,
};

export function spawnToolDefinition(pi: ExtensionAPI, spawn: Spawn): Parameters<ExtensionAPI["registerTool"]>[0] {
	return {
		name: SPAWN_TOOL,
		label: SPAWN_TOOL,
		description: DESCRIPTION,
		parameters: toZod(PARAMETERS, pi.zod),
		defaultInactive: true,
		execute: (_toolCallId: string, params: SpawnParams, signal: AbortSignal | undefined, _onUpdate: unknown, ctx: { cwd: string }) => {
			const cwd = params.cwd === undefined ? ctx.cwd : resolve(ctx.cwd, params.cwd);
			return toolResult(() => spawn({ ...params, cwd }, signal));
		},
	};
}
