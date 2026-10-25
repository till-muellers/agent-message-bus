/**
 * `/bus server start|stop|status`: drives a local agent-message-bus through its
 * CLI (`start`, `stop`, `status --json`, `admin-token`).
 */
import type { CommandResult } from "./shared/command.ts";
import type { CredentialStore } from "./credentials.ts";
import type { Runner } from "./runner.ts";

/** Runs `<binary> <args>` and returns stdout; a non-zero exit is an error. */
export type ServerCli = (args: string[]) => Promise<string>;

export interface ServerCommandDeps {
	cli: ServerCli;
	store: CredentialStore;
	/** Re-activates the bus tools. */
	activate(): Promise<void>;
	/** The full `/bus logout`. */
	logout(): Promise<CommandResult>;
}

export const SERVER_USAGE = "/bus server start|stop|status";

export function createServerCli(runner: Runner, resolveBinary: () => Promise<string>): ServerCli {
	return async args => {
		const bin = await resolveBinary();
		const { code, stdout, stderr } = await runner(bin, args);
		if (code !== 0) throw new Error(`agent-message-bus ${args.join(" ")} failed (exit ${code}): ${(stderr || stdout).trim()}`);
		return stdout;
	};
}

function parseStatus(stdout: string): { running: boolean; url?: string } {
	const status = JSON.parse(stdout) as { running?: unknown; url?: unknown };
	return { running: status.running === true, url: typeof status.url === "string" ? status.url : undefined };
}

/** Starts the server and stores its url and admin token like an admin `/bus login`. */
async function start(deps: ServerCommandDeps): Promise<CommandResult> {
	await deps.cli(["start"]);
	const { running, url } = parseStatus(await deps.cli(["status", "--json"]));
	if (!running || !url) throw new Error("agent-message-bus started but reports no running instance");
	const adminToken = (await deps.cli(["admin-token"])).trim();
	const stored = await deps.store.load().catch(() => undefined);
	await deps.store.save({ url, adminToken, token: stored?.url === url ? stored.token : undefined });
	await deps.activate();
	return { message: `Agent message bus running at ${url}; admin token stored. Run /bus promote <leader-name> to act as a leader.`, level: "info" };
}

async function stop(deps: ServerCommandDeps): Promise<CommandResult> {
	const output = (await deps.cli(["stop"])).trim();
	const { message } = await deps.logout();
	return { message: output ? `${output}\n${message}` : message, level: "info" };
}

export async function serverCommand(sub: string | undefined, deps: ServerCommandDeps): Promise<CommandResult> {
	switch (sub?.toLowerCase()) {
		case "start":
			return await start(deps);
		case "stop":
			return await stop(deps);
		case "status":
			return { message: (await deps.cli(["status"])).trim(), level: "info" };
		default:
			return { message: `Usage: ${SERVER_USAGE}`, level: "error" };
	}
}
