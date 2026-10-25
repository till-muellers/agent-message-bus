import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

/** Opens the worker in a new kitty tab; needs `allow_remote_control` in kitty.conf. */
export const kittyLauncher: Launcher = {
	id: "kitty",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return !!ctx.env.KITTY_WINDOW_ID;
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		return {
			command: "kitty",
			args: ["@", "launch", "--type=tab", "--tab-title", spec.title, "--", ...spec.argv],
			detached: false,
		};
	},
};
