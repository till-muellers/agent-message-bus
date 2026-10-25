import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";
import { posixCommand, powershellCommand } from "./quote.ts";

/** Orca's --command is typed into a shell, so the argv is quoted for that shell. */
function orcaCommand(spec: WorkerSpec, ctx: LaunchContext): LaunchCommand {
	const executable = ctx.env.ORCA_CLI_COMMAND || "orca";
	const shell = ctx.platform === "win32" ? powershellCommand(spec.argv) : posixCommand(spec.argv);
	const args = ["terminal", "create", "--title", spec.title];
	if (ctx.platform === "win32") args.push("--shell", "powershell.exe");
	args.push("--command", shell, "--json");
	return { command: executable, args, detached: false };
}

export const orcaLauncher: Launcher = {
	id: "orca",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return ctx.env.TERM_PROGRAM === "Orca";
	},
	async command(spec: WorkerSpec, ctx: LaunchContext): Promise<LaunchCommand> {
		return orcaCommand(spec, ctx);
	},
};
