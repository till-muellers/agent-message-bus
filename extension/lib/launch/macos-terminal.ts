import type { LaunchCommand, LaunchContext, Launcher, WorkerSpec } from "./launcher.ts";
import { posixCommand } from "./quote.ts";

/** Quotes a POSIX command string for an AppleScript string literal. */
function appleString(s: string): string {
	return `"${s.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

export const macosTerminalLauncher: Launcher = {
	id: "macos-terminal",
	async detect(ctx: LaunchContext): Promise<boolean> {
		return ctx.platform === "darwin";
	},
	async command(spec: WorkerSpec, _ctx: LaunchContext): Promise<LaunchCommand> {
		return {
			command: "osascript",
			args: [
				"-e",
				`tell application "Terminal" to do script ${appleString(posixCommand(spec.argv))}`,
				"-e",
				'tell application "Terminal" to activate',
			],
			detached: false,
		};
	},
};
