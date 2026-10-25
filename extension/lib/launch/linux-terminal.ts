import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";

const TERMINALS: readonly { name: string; args: readonly string[] }[] = [
	{ name: "x-terminal-emulator", args: ["-e"] },
	{ name: "gnome-terminal", args: ["--"] },
	{ name: "konsole", args: ["-e"] },
	{ name: "xterm", args: ["-e"] },
];

async function firstTerminal(ctx: LaunchContext): Promise<{ name: string; args: readonly string[] } | undefined> {
	for (const terminal of TERMINALS) {
		if (await ctx.which(terminal.name)) {
			return terminal;
		}
	}
	return undefined;
}

export const linuxTerminalLauncher: Launcher = {
	id: "linux-terminal",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return ctx.platform === "linux" && !!(ctx.env.DISPLAY || ctx.env.WAYLAND_DISPLAY) && (await firstTerminal(ctx)) !== undefined;
	},
	/** `detect` found a terminal; if PATH changed since, the first one is named and its launch fails instead. */
	async command(spec: WorkerSpec, ctx: LaunchContext): Promise<LaunchCommand> {
		const terminal = (await firstTerminal(ctx)) ?? TERMINALS[0];
		return {
			command: terminal.name,
			args: [...terminal.args, ...spec.argv],
			detached: true,
		};
	},
};
