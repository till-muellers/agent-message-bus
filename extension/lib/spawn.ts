/**
 * One-step worker start: picks a terminal launcher, invites the member (its first prompt travels with the invite) and
 * opens `omp <leader's extension flags> --cwd <cwd> --bus-url <url> --bus-accept <code>`, which joins the space by
 * itself. The code is never returned.
 */
import { BusError, type BusClient } from "./client.ts";
import type { ActiveCredentials } from "./credentials.ts";
import { errorMessage, NOT_LOGGED_IN } from "./errors.ts";
import type { LaunchCommand, LaunchContext, Launcher } from "./launch/launcher.ts";
import { pickLauncher } from "./launch/select.ts";

export interface SpawnDeps {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
	launchers: readonly Launcher[];
	launchContext: LaunchContext;
	/** The leader's extension-loading flags (`lib/plugin-args.ts`), placed right after `omp`. */
	pluginArgs: readonly string[];
	execute(cmd: LaunchCommand, signal?: AbortSignal): Promise<void>;
}

export interface SpawnRequest {
	space: string;
	name: string;
	task?: string;
	cwd: string;
}

export interface SpawnResult {
	name: string;
	space: string;
	launcher: string;
}

async function requireLauncher(deps: SpawnDeps): Promise<Launcher> {
	const launcher = await pickLauncher(deps.launchers, deps.launchContext);
	if (!launcher) throw new Error("no terminal launcher found for this environment");
	return launcher;
}

/** Calls `bus_invite_member` with the task, if any, and returns the invite code. */
async function invite(deps: SpawnDeps, active: ActiveCredentials, { space, name, task }: SpawnRequest): Promise<string> {
	const args = task ? { space, name, task } : { space, name };
	const result = (await deps.client.callTool(active.url, active.token, "bus_invite_member", args)) as { code?: unknown } | null;
	if (typeof result?.code !== "string") throw new BusError("invalid_response", "bus_invite_member: response has no code");
	return result.code;
}

async function launch(deps: SpawnDeps, launcher: Launcher, argv: readonly [string, ...string[]], req: SpawnRequest, signal?: AbortSignal): Promise<void> {
	try {
		await deps.execute(await launcher.command({ title: req.name, argv }, deps.launchContext), signal);
	} catch (error) {
		throw new Error(`${launcher.id}: ${errorMessage(error)}`, { cause: error });
	}
}

export async function spawnWorker(deps: SpawnDeps, req: SpawnRequest, signal?: AbortSignal): Promise<SpawnResult> {
	const active = deps.getActive();
	if (!active) throw new Error(NOT_LOGGED_IN);
	const launcher = await requireLauncher(deps);
	const code = await invite(deps, active, req);
	const argv = ["omp", ...deps.pluginArgs, "--cwd", req.cwd, "--bus-url", active.url, "--bus-accept", code] as const;
	await launch(deps, launcher, argv, req, signal);
	return { name: req.name, space: req.space, launcher: launcher.id };
}
