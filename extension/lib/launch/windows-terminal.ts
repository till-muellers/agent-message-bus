import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

export const windowsTerminalLauncher: Launcher = {
	id: "windows-terminal",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return ctx.platform === "win32" && !!ctx.env.WT_SESSION;
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		// ";" separates commands in Windows Terminal, so it is escaped in every argument.
		const args = spec.argv.map(a => a.replaceAll(";", "\\;"));
		return {
			command: "wt.exe",
			args: ["-w", "0", "new-tab", "--title", spec.title, "--", ...args],
			detached: false,
		};
	},
};
