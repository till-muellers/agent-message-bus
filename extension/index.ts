/**
 * Agent message bus: connects the session to an agent-message-bus server.
 *
 *   /bus login <url> <token>     store an admin, leader or member token
 *   /bus promote <leader-name>   mint a leader token for this session only (admin token required; never stored)
 *   /bus server start|stop|status  run a local agent-message-bus and log in as its admin
 *   /bus status | logout | help
 *
 * Active credentials: the session's promoted leader token, else
 * `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` (read once at load and deleted from the
 * environment so child processes do not inherit them), else the stored agent
 * token. With them the server's `bus_*` tools are registered and the inbox is
 * streamed into the session (delivery policy: `lib/deliver.ts`). Subagents get
 * neither the inbox stream nor the inbox tools. One identity per agent: if
 * another session already streams this agent's inbox (409), the session warns
 * once and keeps retrying. `/bus logout` deactivates every `bus_*` tool until
 * the next `/bus login` or `/bus server start`.
 */
import { join } from "node:path";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { agentConfigDir } from "../_shared/credential-store.ts";
import { activateBusTools, deactivateBusTools } from "./lib/activation.ts";
import { findOnPath, resolveBinary } from "./lib/binary.ts";
import { createClient, type FetchLike } from "./lib/client.ts";
import { runCommand } from "./lib/command.ts";
import { conflictWarning } from "./lib/conflict.ts";
import { type ActiveCredentials, type CredentialStore, createCredentialStore, resolveActive, type SessionCredentials } from "./lib/credentials.ts";
import { createDeliver } from "./lib/deliver.ts";
import { type BusEnv, consumeEnv } from "./lib/env.ts";
import { type InboxLoopOptions, runInboxLoop } from "./lib/inbox.ts";
import { type Runner, spawnRunner } from "./lib/runner.ts";
import { createServerCli, type ServerCli } from "./lib/server.ts";
import { INBOX_TOOLS, registerRemoteTools } from "./lib/tools.ts";

export interface BusExtensionOptions {
	fetchFn?: FetchLike;
	env?: NodeJS.ProcessEnv;
	sleep?: InboxLoopOptions["sleep"];
	/** Runs the agent-message-bus CLI for `/bus server`. */
	runner?: Runner;
	/** Finds `agent-message-bus` on PATH. */
	which?: (name: string) => Promise<string | undefined>;
	platform?: string;
	arch?: string;
}

function describe(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/** The session's bus connection: session/active credentials, registered tools and the inbox loop. */
function createSession(pi: ExtensionAPI, store: CredentialStore, busEnv: BusEnv, options: BusExtensionOptions) {
	const client = createClient(options.fetchFn);
	const warn = (message: string) => pi.logger.warn(`agent-bus: ${message}`);
	const state = { registered: false, names: [] as string[] };
	const credentials: SessionCredentials = { env: busEnv.credentials };
	let active: ActiveCredentials | undefined;
	let loop: AbortController | undefined;
	let mode: string | undefined;
	let sub = false;
	const getActive = () => active;
	const deliver = createDeliver(pi, () => mode);

	function stop(): void {
		loop?.abort();
		loop = undefined;
	}

	async function warnConflict(): Promise<void> {
		const current = getActive();
		if (current) warn(await conflictWarning(client, current));
	}

	function startLoop(): void {
		loop = new AbortController();
		const onError = (error: Error) => warn(error.message);
		void runInboxLoop({ client, getActive, signal: loop.signal, sleep: options.sleep, deliver, onError, onConflict: warnConflict }).catch(error =>
			warn(describe(error)),
		);
	}

	/** Subagents register no inbox tools and run no inbox loop: their parent session owns the inbox. */
	async function restart(): Promise<void> {
		stop();
		active = resolveActive(credentials, await store.load());
		if (!active) return;
		await registerRemoteTools(pi, { client, getActive, state, warn, exclude: sub ? INBOX_TOOLS : undefined });
		if (!sub) startLoop();
	}

	return {
		client,
		credentials,
		warn,
		restart,
		/** Session end: stops the loop and forgets the promoted leader token. */
		shutdown(): void {
			stop();
			credentials.override = undefined;
		},
		/** Stops the loop, drops the credentials and deactivates every bus tool; registered tools stay and fail until the next login. */
		async logout(): Promise<void> {
			stop();
			active = undefined;
			await deactivateBusTools(pi);
		},
		activate: () => activateBusTools(pi, state.names),
		setContext(ctx: { mode?: string; agent?: { kind?: string } }): void {
			mode = ctx.mode;
			sub = ctx.agent?.kind === "sub";
		},
	};
}

/** The local agent-message-bus CLI: `AGENT_BUS_BIN`, else PATH, else the verified release download. */
function localServerCli(env: NodeJS.ProcessEnv, busEnv: BusEnv, options: BusExtensionOptions): ServerCli {
	const binary = () =>
		resolveBinary({
			bin: busEnv.bin,
			which: options.which ?? (name => findOnPath(name, env.PATH)),
			fetchFn: options.fetchFn ?? fetch,
			dir: join(agentConfigDir(env), "agent-message-bus", "bin"),
			platform: options.platform ?? process.platform,
			arch: options.arch ?? process.arch,
		});
	return createServerCli(options.runner ?? spawnRunner, binary);
}

export function createBusExtension(options: BusExtensionOptions = {}): (pi: ExtensionAPI) => Promise<void> {
	return async pi => {
		const env = options.env ?? process.env;
		const busEnv = consumeEnv(env);
		const store = createCredentialStore(env);
		const session = createSession(pi, store, busEnv, options);
		const server = localServerCli(env, busEnv, options);

		pi.registerCommand("bus", {
			description: "Agent message bus: login <url> <token> | promote <leader-name> | server start|stop|status | status | logout | help",
			handler: async (args: string | string[] | undefined, ctx) => {
				session.setContext(ctx);
				const { client, credentials, restart, activate, logout } = session;
				const deps = { store, client, session: credentials, server, restart, activate, stop: logout };
				const { message, level } = await runCommand(args, deps);
				ctx.ui.notify(message, level);
			},
		});

		pi.on("session_start", async (_event, ctx) => {
			session.setContext(ctx);
			try {
				await session.restart();
			} catch (error) {
				session.warn(describe(error));
			}
		});
		pi.on("session_shutdown", () => session.shutdown());
	};
}

export default createBusExtension();
