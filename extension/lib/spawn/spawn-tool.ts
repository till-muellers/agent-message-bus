/**
 * `bus_spawn_worker`: the agent-facing form of `/bus spawn`, a local tool next to the server's `bus_*` tools. It is
 * registered inactive and switched on with them once the session has credentials. Its call renders as an outgoing
 * card: the task reaches the worker as a direct message from the caller.
 */
import { resolve } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { type JsonSchema, toZod } from "../bus/schema.ts";
import { argText, fixedSender, outgoingCard } from "../render/outgoing-card.ts";
import type { SpawnRequest, SpawnResult } from "./spawn.ts";
import { presentationOf } from "../tools/tool-policy.ts";
import { toolResult } from "../tools/tool-result.ts";

export const SPAWN_TOOL = "bus_spawn_worker";

interface SpawnParams {
	space: string;
	name: string;
	task?: string;
	cwd?: string;
}

export type Spawn = (req: SpawnRequest, signal?: AbortSignal) => Promise<SpawnResult>;

/** The documented description (`server/internal/tools/ledger_tail.md.tmpl`, `tool-descriptions.feature`). */
const DESCRIPTION =
	"Starts a worker agent: invites member `<name>` and starts it as a separate omp session in a new terminal tab (Orca, tmux, zellij, WezTerm, kitty, Windows Terminal; else a new window). The worker joins the space by itself; the invite code is never exposed. Pitfalls: the task is delivered to the worker as a direct message from the caller (a leader or an admin); each worker needs its own name; a name already used by a leader agent fails with conflict, and without a terminal launcher the leader invites by hand. Leaders of the space and admins.";

const PARAMETERS: JsonSchema = {
	type: "object",
	properties: {
		space: { type: "string", description: "Space name." },
		name: { type: "string", description: "Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$." },
		task: { type: "string", description: "Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report." },
		cwd: { type: "string", description: "Working directory, relative to this session's; defaults to this session's." },
	},
	required: ["space", "name"],
	additionalProperties: false,
};

/** `self` names the bus agent the session acts as; undefined while unknown. */
export function spawnToolDefinition(pi: ExtensionAPI, spawn: Spawn, self: () => string | undefined): Parameters<ExtensionAPI["registerTool"]>[0] {
	const sender = fixedSender(self);
	return {
		name: SPAWN_TOOL,
		label: SPAWN_TOOL,
		description: DESCRIPTION,
		parameters: toZod(PARAMETERS, pi.zod),
		...presentationOf(SPAWN_TOOL),
		defaultInactive: true,
		renderCall: (args, { expanded, isPartial }, theme) => {
			const message = { from: sender(args, isPartial), to: argText(args, "name"), space: argText(args, "space"), where: "spawn", body: argText(args, "task") };
			return outgoingCard(pi.pi, message, expanded, theme);
		},
		execute: (_toolCallId: string, params: SpawnParams, signal: AbortSignal | undefined, _onUpdate: unknown, ctx: { cwd: string }) => {
			const cwd = params.cwd === undefined ? ctx.cwd : resolve(ctx.cwd, params.cwd);
			return toolResult(() => spawn({ ...params, cwd }, signal));
		},
	};
}
