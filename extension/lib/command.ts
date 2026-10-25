/**
 * `/bus` command logic, kept free of the omp API so it is unit-testable: the
 * handler gets the credential store, the HTTP client and the loop controls,
 * and returns the message to show.
 */
import { type CommandResult, parseArgs } from "./shared/command.ts";
import { BusError, type BusClient } from "./client.ts";
import { type CredentialStore, resolveActive, type SessionCredentials, trimUrl } from "./credentials.ts";
import { type ServerCli, serverCommand } from "./server.ts";

export interface CommandDeps {
	store: CredentialStore;
	client: BusClient;
	/** In-memory credentials of this session: `/bus promote` sets the override, login (agent token) clears it, logout clears it and the env credentials. */
	session: SessionCredentials;
	/** Runs the local agent-message-bus CLI for `/bus server`. */
	server: ServerCli;
	/** Re-resolves credentials, registers tools and restarts the inbox loop. */
	restart(): Promise<void>;
	/** Re-activates the bus tools after a logout. */
	activate(): Promise<void>;
	/** Stops the inbox loop, drops the active credentials and deactivates every `bus_*` tool. */
	stop(): Promise<void>;
}

interface Whoami {
	kind: "admin" | "leader" | "member";
	name?: string;
	space?: string;
	spaces: { name: string; role: string }[];
}

export const USAGE = [
	"Agent message bus:",
	"  /bus login <url> <token>     store a bus token (admin, leader or member) and activate the bus tools",
	"  /bus promote <leader-name>   mint a leader token with the stored admin token; this session only, never stored",
	"  /bus status                  show who this session is on the bus",
	"  /bus logout                  forget the stored, session and env credentials, stop the inbox stream and deactivate the bus tools",
	"  /bus server start            start a local agent-message-bus (AGENT_BUS_BIN, PATH or download) and store its admin token",
	"  /bus server stop             stop the local agent-message-bus and log out",
	"  /bus server status           show whether the local agent-message-bus is running",
	"  /bus help                    show this help",
].join("\n");

async function status(deps: CommandDeps): Promise<CommandResult> {
	const stored = await deps.store.load();
	const active = resolveActive(deps.session, stored);
	const url = active?.url ?? (stored ? trimUrl(stored.url) : undefined);
	const token = active?.token ?? stored?.adminToken;
	if (!url || !token) return { message: `Not logged in.\n${USAGE}`, level: "info" };
	const me = (await deps.client.callTool(url, token, "bus_whoami", {})) as Whoami;
	const spaces = me.spaces.map(s => `${s.name} (${s.role})`).join(", ") || "none";
	return { message: `${url}: ${me.kind}${me.name ? ` ${me.name}` : ""}; spaces: ${spaces}`, level: "info" };
}

async function login(deps: CommandDeps, rawUrl: string | undefined, token: string | undefined): Promise<CommandResult> {
	if (!rawUrl || !token) return { message: USAGE, level: "error" };
	const url = trimUrl(rawUrl);
	const me = (await deps.client.callTool(url, token, "bus_whoami", {})) as Whoami;
	const stored = await deps.store.load().catch(() => undefined);
	if (me.kind === "admin") {
		await deps.store.save({ url, adminToken: token, token: stored?.url === url ? stored.token : undefined });
		await deps.activate();
		return { message: "Admin token stored. Run /bus promote <leader-name> to act as a leader.", level: "info" };
	}
	await deps.store.save({ url, adminToken: stored?.url === url ? stored.adminToken : undefined, token });
	deps.session.override = undefined;
	await deps.restart();
	await deps.activate();
	return { message: `Logged in as ${me.kind} ${me.name}`, level: "info" };
}

/** Mints a leader token held in this session only: credentials.json keeps just the url and admin token. */
async function promote(deps: CommandDeps, name: string | undefined): Promise<CommandResult> {
	if (!name) return { message: USAGE, level: "error" };
	const stored = await deps.store.load();
	if (!stored?.adminToken) return { message: "Run /bus login <url> <admin-token> first.", level: "error" };
	const url = trimUrl(stored.url);
	const minted = (await deps.client.callTool(url, stored.adminToken, "bus_mint_leader", { name })) as { name: string; token: string };
	deps.session.override = { url, token: minted.token };
	await deps.restart();
	return { message: `This session now acts as leader ${minted.name} (until it ends; run /bus promote again after a restart).`, level: "info" };
}

/** Clears the stored file, the session override and the env credentials; no fallback remains, so the bus tools are deactivated. */
async function logout(deps: CommandDeps): Promise<CommandResult> {
	await deps.store.clear();
	deps.session.override = undefined;
	deps.session.env = undefined;
	await deps.stop();
	return { message: "Logged out; bus tools deactivated.", level: "info" };
}

export async function runCommand(rawArgs: string | string[] | undefined, deps: CommandDeps): Promise<CommandResult> {
	const [sub = "status", ...rest] = parseArgs(rawArgs);
	try {
		switch (sub.toLowerCase()) {
			case "status":
				return await status(deps);
			case "login":
				return await login(deps, rest[0], rest[1]);
			case "promote":
				return await promote(deps, rest[0]);
			case "logout":
				return await logout(deps);
			case "server":
				return await serverCommand(rest[0], { cli: deps.server, store: deps.store, activate: deps.activate, logout: () => logout(deps) });
			case "help":
				return { message: USAGE, level: "info" };
			default:
				return { message: `Unknown subcommand "${sub}".\n${USAGE}`, level: "error" };
		}
	} catch (error) {
		if (error instanceof BusError) return { message: `${error.code}: ${error.message}`, level: "error" };
		return { message: error instanceof Error ? error.message : String(error), level: "error" };
	}
}
