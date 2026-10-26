/**
 * Agent message bus: connects the session to an agent-message-bus server.
 *
 *   /bus login <url> <token>     store an admin, leader or member token
 *   /bus accept <url> <code>     redeem an invite code; the member token is kept for this omp session
 *   /bus spawn <space> <name> [task]  invite a member and open it in a new terminal tab (`lib/spawn.ts`; also the bus_spawn_worker tool)
 *   /bus promote <leader-name>   mint a leader token for this session only (admin token required; never stored)
 *   /bus server start|stop|status  run a local agent-message-bus and log in as its admin
 *   /bus status | logout | help
 *   omp --bus-url <url> --bus-accept <code>  redeem an invite code when the session starts (`lib/invite-flag.ts`)
 *
 * Active credentials: the session's promoted leader token, else
 * `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` (read once at load and deleted from the
 * environment so child processes do not inherit them), else the member token
 * this session accepted (`lib/session-identity.ts`), else the stored agent
 * token. With them the server's `bus_*` tools are registered and, with
 * `bus_spawn_worker`, switched on, and the inbox is streamed into the session;
 * without them those tools are switched off (`lib/connection.ts`; delivery
 * policy: `lib/deliver.ts`). Session start, switch and branch re-key the
 * accepted token (a branch or fork carries it to its new session file) and
 * reconnect. Subagents get neither the inbox stream nor the inbox tools.
 * `/bus logout` deactivates every `bus_*` tool until agent credentials resolve again.
 */
import { join } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { agentConfigDir } from "./lib/shared/credential-store.ts";
import type { CommandResult } from "./lib/shared/command.ts";
import { acceptInvite, type Joined } from "./lib/accept.ts";
import { findOnPath, resolveBinary } from "./lib/binary.ts";
import { type BusClient, createClient, type FetchLike } from "./lib/bus/client.ts";
import { COMMAND_DESCRIPTION, runCommand } from "./lib/command.ts";
import { type Connection, createConnection, type SessionView } from "./lib/connection.ts";
import { type CredentialStore, configDir, createCredentialStore, type SessionCredentials } from "./lib/session/stored-credentials.ts";
import { type BusEnv, consumeEnv } from "./lib/session/env.ts";
import { errorMessage } from "./lib/bus/errors.ts";
import type { InboxLoopOptions } from "./lib/inbox.ts";
import { createInviteFlag } from "./lib/invite-flag.ts";
import { type DetachedSpawner, spawnDetached } from "./lib/launch/detached.ts";
import { LAUNCHERS } from "./lib/launch/index.ts";
import type { LaunchCommand, LaunchContext } from "./lib/launch/launcher.ts";
import { executeLaunch } from "./lib/launch/run.ts";
import { pluginArgs } from "./lib/plugin-args.ts";
import { type Runner, spawnRunner } from "./lib/runner.ts";
import { createServerCli, type ServerCli } from "./lib/server.ts";
import { createSessionIdentity, type IdentityContext, type SessionChange } from "./lib/session/session-identity.ts";
import { spawnWorker } from "./lib/spawn.ts";
import { SPAWN_TOOL, type Spawn, spawnToolDefinition } from "./lib/spawn-tool.ts";

export interface BusExtensionOptions {
	fetchFn?: FetchLike;
	env?: NodeJS.ProcessEnv;
	sleep?: InboxLoopOptions["sleep"];
	/** Runs the agent-message-bus CLI for `/bus server` and the terminal launchers of `/bus spawn`. */
	runner?: Runner;
	/** Starts launchers that may block until their window closes (`linux-terminal`). */
	spawnDetached?: DetachedSpawner;
	/** Finds an executable on PATH: `agent-message-bus`, and the terminals a launcher probes. */
	which?: (name: string) => Promise<string | undefined>;
	platform?: string;
	arch?: string;
	/** This omp process's argv, whose extension-loading flags a spawned worker gets; default `process.argv`. */
	argv?: readonly string[];
}

/** The parts of the omp context a session reads: mode, agent kind and the session file. */
interface SessionContext extends IdentityContext {
	mode?: string;
}

/** The session's bus state: its credentials, accepted identity and connection. */
interface Session extends Connection {
	client: BusClient;
	credentials: SessionCredentials;
	warn(message: string): void;
	isSub(): boolean;
	clearAccepted(): Promise<void>;
	accept(url: string, code: string): Promise<Joined>;
	/** After an accept: reconnects as the member, which switches the tools on, and sends the invite's first prompt. */
	join(joined: Joined): Promise<CommandResult>;
	/** Session end: stops the loop and forgets the promoted leader token. */
	shutdown(): void;
	setContext(ctx: SessionContext, change?: SessionChange): Promise<void>;
}

/** `dir` holds the per-session files. */
function createSession(pi: ExtensionAPI, store: CredentialStore, busEnv: BusEnv, options: BusExtensionOptions, dir: string): Session {
	const client = createClient(options.fetchFn);
	const warn = (message: string) => pi.logger.warn(`agent-bus: ${message}`);
	const credentials: SessionCredentials = { env: busEnv.credentials };
	const identity = createSessionIdentity(dir, credentials);
	const view: SessionView = { sub: false };
	const connection = createConnection({ pi, client, store, credentials, identity, view, warn, sleep: options.sleep, localTools: [SPAWN_TOOL] });
	return {
		...connection,
		client,
		credentials,
		warn,
		isSub: () => view.sub,
		clearAccepted: () => identity.clear(),
		accept: (url, code) => acceptInvite({ client, session: credentials, identity }, url, code),
		async join(joined) {
			await connection.restart();
			if (joined.task) pi.sendUserMessage(joined.task);
			return joined.result;
		},
		shutdown() {
			connection.stop();
			credentials.override = undefined;
		},
		setContext(ctx, change) {
			view.mode = ctx.mode;
			view.sub = ctx.agent?.kind === "sub";
			return identity.bind(ctx, change);
		},
	};
}

/** The local agent-message-bus CLI: `AGENT_BUS_BIN`, else PATH, else the verified release download. */
function localServerCli(busEnv: BusEnv, options: BusExtensionOptions, which: LaunchContext["which"], env: NodeJS.ProcessEnv): ServerCli {
	const binary = () =>
		resolveBinary({
			bin: busEnv.bin,
			which,
			fetchFn: options.fetchFn ?? fetch,
			dir: join(agentConfigDir(env), "agent-message-bus", "bin"),
			platform: options.platform ?? process.platform,
			arch: options.arch ?? process.arch,
		});
	return createServerCli(options.runner ?? spawnRunner, binary);
}

/** `/bus spawn` and `bus_spawn_worker`: the first launcher detected in `launchContext` opens the worker. */
function workerSpawner(launchContext: LaunchContext, options: BusExtensionOptions, session: Session): Spawn {
	const launch = { runner: options.runner ?? spawnRunner, spawnDetached: options.spawnDetached ?? spawnDetached };
	const deps = {
		client: session.client,
		getActive: session.getActive,
		launchers: LAUNCHERS,
		launchContext,
		pluginArgs: pluginArgs(options.argv ?? process.argv),
		execute: (cmd: LaunchCommand, signal?: AbortSignal) => executeLaunch(cmd, launch, signal),
	};
	return (req, signal) => spawnWorker(deps, req, signal);
}

function registerBusCommand(pi: ExtensionAPI, store: CredentialStore, session: Session, server: ServerCli, spawn: Spawn): void {
	pi.registerCommand("bus", {
		description: COMMAND_DESCRIPTION,
		handler: async (args: string | string[] | undefined, ctx) => {
			await session.setContext(ctx);
			const { client, credentials, restart, logout, clearAccepted } = session;
			const accept = async (url: string, code: string) => session.join(await session.accept(url, code));
			const spawnHere = (req: { space: string; name: string; task?: string }) => spawn({ ...req, cwd: ctx.cwd });
			const deps = { store, client, session: credentials, server, restart, stop: logout, accept, clearAccepted, spawn: spawnHere };
			const { message, level } = await runCommand(args, deps);
			ctx.ui.notify(message, level);
		},
	});
}

/** Session start redeems `--bus-accept` once; start, switch and branch re-key the accepted token and reconnect. */
function registerSessionEvents(pi: ExtensionAPI, store: CredentialStore, session: Session): void {
	const redeemFlag = createInviteFlag({
		flag: name => pi.getFlag(name),
		isSub: session.isSub,
		storedUrl: async () => (await store.load())?.url,
		accept: session.accept,
		warn: session.warn,
	});
	const guarded = (work: () => Promise<void>) => work().catch(error => session.warn(errorMessage(error)));
	pi.on("session_start", (_event, ctx) =>
		guarded(async () => {
			await session.setContext(ctx);
			const joined = await redeemFlag();
			if (!joined) return session.restart();
			const { message, level } = await session.join(joined);
			ctx.ui.notify(message, level);
		}),
	);
	const reconnect = (ctx: SessionContext, change: SessionChange) =>
		guarded(async () => {
			await session.setContext(ctx, change);
			await session.restart();
		});
	pi.on("session_switch", ({ reason, previousSessionFile }, ctx) =>
		reconnect(ctx, reason === "fork" ? { kind: "branch", previousSessionFile } : { kind: "new" }),
	);
	pi.on("session_branch", ({ previousSessionFile }, ctx) => reconnect(ctx, { kind: "branch", previousSessionFile }));
	pi.on("session_shutdown", () => session.shutdown());
}

export function createBusExtension(options: BusExtensionOptions = {}): (pi: ExtensionAPI) => Promise<void> {
	return async pi => {
		const env = options.env ?? process.env;
		const busEnv = consumeEnv(env);
		const store = createCredentialStore(env);
		const session = createSession(pi, store, busEnv, options, configDir(env));
		const which = options.which ?? ((name: string) => findOnPath(name, env.PATH));
		const spawn = workerSpawner({ env, platform: options.platform ?? process.platform, which }, options, session);
		pi.registerFlag("bus-url", { type: "string", description: "Bus url for --bus-accept" });
		pi.registerFlag("bus-accept", { type: "string", description: "Invite code to redeem when the session starts" });
		pi.registerTool(spawnToolDefinition(pi, spawn));
		registerBusCommand(pi, store, session, localServerCli(busEnv, options, which, env), spawn);
		registerSessionEvents(pi, store, session);
	};
}

export default createBusExtension();
