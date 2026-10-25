import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";
import { psQuote } from "./quote.ts";

/** Doubles trailing backslashes so they don't escape the closing double quote. */
function quoteArgument(a: string): string {
	return psQuote(`"${a.replace(/\\+$/, m => m + m)}"`);
}

export const windowsConsoleLauncher: Launcher = {
	id: "windows-console",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return ctx.platform === "win32";
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		// PowerShell 5.1 does not quote ArgumentList items, so each one carries inner double quotes.
		const argumentList = spec.argv.slice(1).map(quoteArgument).join(",");
		const script = `Start-Process -FilePath ${psQuote(spec.argv[0])} -ArgumentList ${argumentList}`;
		return {
			command: "powershell.exe",
			args: ["-NoProfile", "-NonInteractive", "-Command", script],
			detached: false,
		};
	},
};
