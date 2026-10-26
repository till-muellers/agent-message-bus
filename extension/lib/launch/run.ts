import type { Runner } from "../process/runner.ts";
import type { DetachedSpawner } from "./detached.ts";
import type { LaunchCommand } from "./launcher.ts";

/** Launcher CLIs return once the tab is open; one that waits longer (a remote-control prompt, an unreachable mux) is killed. */
export const LAUNCH_TIMEOUT_MS = 15_000;

interface ExecuteDeps {
	runner: Runner;
	spawnDetached: DetachedSpawner;
}

/** Runs a launch command; a non-zero exit becomes an error carrying the CLI's output. `signal` kills a running CLI. */
export async function executeLaunch(cmd: LaunchCommand, deps: ExecuteDeps, signal?: AbortSignal): Promise<void> {
	if (cmd.detached) return deps.spawnDetached(cmd.command, cmd.args);
	const { code, stdout, stderr } = await deps.runner(cmd.command, cmd.args, { timeoutMs: LAUNCH_TIMEOUT_MS, signal });
	if (code !== 0) throw new Error(`${cmd.command} exited with ${code}: ${(stderr || stdout).trim()}`);
}
