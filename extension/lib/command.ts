/**
 * `/bus` command logic, kept free of the omp API so it is unit-testable: the
 * handler gets the credential store, the HTTP client and the loop controls,
 * and returns the message to show.
 */
import { BusError, type BusClient } from "./client.ts";
import { type CredentialStore, resolveActive, trimUrl } from "./credentials.ts";

export interface CommandDeps {
	store: CredentialStore;
	client: BusClient;
	env: NodeJS.ProcessEnv;
	/** Re-resolves credentials, registers tools and restarts the inbox loop. */
	restart(): Promise<void>;
	/** Stops the inbox loop. */
	stop(): void;
}

export interface CommandResult {
	message: string;
	level: "info" | "warning" | "error";
}

interface Whoami {
	kind: "admin" | "leader" | "member";
	name?: string;
	space?: string;
	spaces: { name: string; role: string }[];
}

export const USAGE = [
	"Agent message bus:",
	"  /bus login <url> <token>     store a bus token (admin, leader or member)",
	"  /bus promote <leader-name>   mint a leader token with the stored admin token and use it",
	"  /bus status                  show who this session is on the bus",
	"  /bus logout                  forget the stored credentials and stop the inbox stream",
].join("\n");

function parseArgs(args: string | string[] | undefined): string[] {
	if (Array.isArray(args)) return args.map(String).filter(Boolean);
	return String(args ?? "").trim().split(/\s+/).filter(Boolean);
}

async function status(deps: CommandDeps): Promise<CommandResult> {
	const stored = await deps.store.load();
	const active = resolveActive(deps.env, stored);
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
		return { message: "Admin token stored. Run /bus promote <leader-name> to act as a leader.", level: "info" };
	}
	await deps.store.save({ ...stored, url, token });
	await deps.restart();
	return { message: `Logged in as ${me.kind} ${me.name}`, level: "info" };
}

async function promote(deps: CommandDeps, name: string | undefined): Promise<CommandResult> {
	if (!name) return { message: USAGE, level: "error" };
	const stored = await deps.store.load();
	if (!stored?.adminToken) return { message: "Run /bus login <url> <admin-token> first.", level: "error" };
	const url = trimUrl(stored.url);
	const minted = (await deps.client.callTool(url, stored.adminToken, "bus_mint_leader", { name })) as { name: string; token: string };
	await deps.store.save({ ...stored, url, token: minted.token });
	await deps.restart();
	return { message: `This session now acts as leader ${minted.name}.`, level: "info" };
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
				deps.stop();
				await deps.store.clear();
				return { message: "Logged out.", level: "info" };
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
