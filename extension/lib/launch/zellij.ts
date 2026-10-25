import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

export const zellijLauncher: Launcher = {
	id: "zellij",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return !!ctx.env.ZELLIJ;
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		return {
			command: "zellij",
			args: ["action", "new-tab", "--name", spec.title, "--", ...spec.argv],
			detached: false,
		};
	},
};
