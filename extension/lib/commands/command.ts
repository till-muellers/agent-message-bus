/**
 * `/bus` command logic, kept free of the omp API so it is unit-testable: the
 * handler gets the credential store, the HTTP client and the loop controls,
 * and returns the message to show. `SUBCOMMANDS` is the one list the usage
 * text, the command description and the dispatch are built from.
 */
import { type CommandResult, parseArgs } from "../shared/command.ts";
import type { BusClient } from "../bus/client.ts";
import { type CredentialStore, resolveActive, type SessionCredentials, trimUrl } from "../session/stored-credentials.ts";
import { errorMessage } from "../bus/errors.ts";
import { type ServerCli, serverCommand } from "../server.ts";
import type { SpawnRequest, SpawnResult } from "../spawn.ts";

export interface CommandDeps {
	store: CredentialStore;
	client: BusClient;
	/** In-memory credentials of this session: `/bus promote` sets the override, login (agent token) clears it, logout clears it and the env credentials. */
	session: SessionCredentials;
	/** Runs the local agent-message-bus CLI for `/bus server`. */
	server: ServerCli;
	/** Re-resolves credentials, registers tools and restarts the inbox loop. */
	restart(): Promise<void>;
	/** Stops the inbox loop, drops the active credentials and deactivates every `bus_*` tool. */
	stop(): Promise<void>;
	/** `/bus accept <url> <code>`: redeems the invite code, joins its space for this session and starts its first prompt. */
	accept(url: string, code: string): Promise<CommandResult>;
	/** Forgets the token accepted by `/bus accept` in memory and in this session's credential file. */
	clearAccepted(): Promise<void>;
	/** `/bus spawn <space> <name> [task…]`: invites the member and starts it in a new terminal tab (`lib/spawn/spawn.ts`); cwd is the session's. */
	spawn(req: Omit<SpawnRequest, "cwd">): Promise<SpawnResult>;
}

interface Whoami {
	kind: "admin" | "leader" | "member";
	name?: string;
	space?: string;
	spaces: { name: string; role: string }[];
}

interface Subcommand {
	name: string;
	args: string;
	summary: string;
	run(deps: CommandDeps, args: string[]): Promise<CommandResult>;
}

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

async function login(deps: CommandDeps, [rawUrl, token]: string[]): Promise<CommandResult> {
	if (!rawUrl || !token) return usageError();
	const url = trimUrl(rawUrl);
	const me = (await deps.client.callTool(url, token, "bus_whoami", {})) as Whoami;
	const stored = await deps.store.load().catch(() => undefined);
	if (me.kind === "admin") {
		await deps.store.save({ url, adminToken: token, token: stored?.url === url ? stored.token : undefined });
		await deps.restart();
		return { message: "Admin token stored. Run /bus promote <leader-name> to act as a leader.", level: "info" };
	}
	await deps.store.save({ url, adminToken: stored?.url === url ? stored.adminToken : undefined, token });
	deps.session.override = undefined;
	await deps.clearAccepted();
	await deps.restart();
	return { message: `Logged in as ${me.kind} ${me.name}`, level: "info" };
}

/** Mints a leader token held in this session only: credentials.json keeps just the url and admin token. */
async function promote(deps: CommandDeps, [name]: string[]): Promise<CommandResult> {
	if (!name) return usageError();
	const stored = await deps.store.load();
	if (!stored?.adminToken) return { message: "Run /bus login <url> <admin-token> first.", level: "error" };
	const url = trimUrl(stored.url);
	const minted = (await deps.client.callTool(url, stored.adminToken, "bus_mint_leader", { name })) as { name: string; token: string };
	deps.session.override = { url, token: minted.token };
	await deps.restart();
	return { message: `This session now acts as leader ${minted.name} (until it ends; run /bus promote again after a restart).`, level: "info" };
}

/** Clears the stored file, the accepted token, the session override and the env credentials; no fallback remains, so the bus tools are deactivated. */
async function logout(deps: CommandDeps): Promise<CommandResult> {
	await deps.store.clear();
	await deps.clearAccepted();
	deps.session.override = undefined;
	deps.session.env = undefined;
	await deps.stop();
	return { message: "Logged out; bus tools deactivated.", level: "info" };
}

async function spawn(deps: CommandDeps, [space, name, ...task]: string[]): Promise<CommandResult> {
	if (!space || !name) return usageError();
	const started = await deps.spawn({ space, name, task: task.join(" ") || undefined });
	return { message: `Started worker ${started.name} in space ${started.space} (${started.launcher}).`, level: "info" };
}

const SUBCOMMANDS: readonly Subcommand[] = [
	{ name: "login", args: "<url> <token>", summary: "store a bus token (admin, leader or member) and activate the bus tools", run: login },
	{
		name: "accept",
		args: "<url> <code>",
		summary: "redeem an invite code: join its space as that member (stored for this session)",
		run: async (deps, [url, code]) => (url && code ? deps.accept(url, code) : usageError()),
	},
	{ name: "spawn", args: "<space> <name> [task]", summary: "invite a new member and start it in a new terminal tab; it joins by itself", run: spawn },
	{ name: "promote", args: "<leader-name>", summary: "mint a leader token with the stored admin token; this session only, never stored", run: promote },
	{
		name: "server",
		args: "start|stop|status",
		summary: "start a local agent-message-bus (AGENT_BUS_BIN, PATH or download) and store its admin token; stop it and log out; or show whether it runs",
		run: (deps, [sub]) => serverCommand(sub, { cli: deps.server, store: deps.store, restart: deps.restart, logout: () => logout(deps) }),
	},
	{ name: "status", args: "", summary: "show who this session is on the bus", run: status },
	{
		name: "logout",
		args: "",
		summary: "forget the stored, accepted, session and env credentials, stop the inbox stream and deactivate the bus tools",
		run: logout,
	},
	{ name: "help", args: "", summary: "show this help", run: async () => ({ message: USAGE, level: "info" }) },
];

const usageOf = (s: Subcommand) => `/bus ${s.name}${s.args ? ` ${s.args}` : ""}`;
const WIDTH = Math.max(...SUBCOMMANDS.map(s => usageOf(s).length)) + 2;

export const USAGE = ["Agent message bus:", ...SUBCOMMANDS.map(s => `  ${usageOf(s).padEnd(WIDTH)}${s.summary}`)].join("\n");

/** The registered `/bus` command's description. */
export const COMMAND_DESCRIPTION = `Agent message bus: ${SUBCOMMANDS.map(s => usageOf(s).slice("/bus ".length)).join(" | ")}`;

function usageError(): CommandResult {
	return { message: USAGE, level: "error" };
}

export async function runCommand(rawArgs: string | string[] | undefined, deps: CommandDeps): Promise<CommandResult> {
	const [sub = "status", ...rest] = parseArgs(rawArgs);
	const subcommand = SUBCOMMANDS.find(s => s.name === sub.toLowerCase());
	if (!subcommand) return { message: `Unknown subcommand "${sub}".\n${USAGE}`, level: "error" };
	try {
		return await subcommand.run(deps, rest);
	} catch (error) {
		return { message: errorMessage(error), level: "error" };
	}
}
