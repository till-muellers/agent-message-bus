/** The contract every terminal launcher implements: detect its environment, build the command that opens a worker. */

/** The worker to open: a tab/window title and the omp argv (argv[0] === "omp"). */
export interface WorkerSpec {
	title: string;
	argv: readonly [string, ...string[]];
}

/** A process to run; `detached` ones may block until their window closes, so they are not awaited. */
export interface LaunchCommand {
	command: string;
	args: string[];
	detached: boolean;
}

export interface LaunchContext {
	env: NodeJS.ProcessEnv;
	platform: string;
	which(name: string): Promise<string | undefined>;
}

/** All environment probing (env, platform, PATH) happens in `detect`; `command()` never fails because of the environment. */
export interface Launcher {
	readonly id: string;
	detect(ctx: LaunchContext): Promise<boolean>;
	command(spec: WorkerSpec, ctx: LaunchContext): Promise<LaunchCommand>;
}
