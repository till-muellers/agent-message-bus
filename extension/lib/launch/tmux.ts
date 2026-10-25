import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

export const tmuxLauncher: Launcher = {
	id: "tmux",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return !!ctx.env.TMUX;
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		return {
			command: "tmux",
			args: ["new-window", "-n", spec.title, "--", ...spec.argv],
			detached: false,
		};
	},
};
