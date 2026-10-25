import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

export const weztermLauncher: Launcher = {
	id: "wezterm",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return !!ctx.env.WEZTERM_PANE;
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		return {
			command: "wezterm",
			args: ["cli", "spawn", "--", ...spec.argv],
			detached: false,
		};
	},
};
