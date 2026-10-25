/**
 * Agent message bus: connects the session to an agent-message-bus server.
 *
 *   /bus login <url> <token>     store an admin, leader or member token
 *   /bus promote <leader-name>   mint a leader token for this session only (admin token required; never stored)
 *   /bus status | logout | help
 *
 * Active credentials: the session's promoted leader token, else env
 * `AGENT_BUS_URL` + `AGENT_BUS_TOKEN`, else the stored agent token. With them
 * the server's `bus_*` tools are registered and the inbox is streamed into the
 * session (delivery policy: `lib/deliver.ts`). One identity per agent: if
 * another session already streams this agent's inbox (409), the session warns
 * once and keeps retrying.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createClient, type FetchLike } from "./lib/client.ts";
import { runCommand } from "./lib/command.ts";
import { conflictWarning } from "./lib/conflict.ts";
import {
	type ActiveCredentials,
	type CredentialStore,
	createCredentialStore,
	resolveActive,
	type SessionCredentials,
} from "./lib/credentials.ts";
import { createDeliver } from "./lib/deliver.ts";
import { type InboxLoopOptions, runInboxLoop } from "./lib/inbox.ts";
import { registerRemoteTools } from "./lib/tools.ts";

export interface BusExtensionOptions {
	fetchFn?: FetchLike;
	env?: NodeJS.ProcessEnv;
	sleep?: InboxLoopOptions["sleep"];
}

function describe(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/** The session's bus connection: session/active credentials, registered tools and the inbox loop. */
function createSession(pi: ExtensionAPI, env: NodeJS.ProcessEnv, store: CredentialStore, options: BusExtensionOptions) {
	const client = createClient(options.fetchFn);
	const warn = (message: string) => pi.logger.warn(`agent-bus: ${message}`);
	const state = { registered: false };
	const credentials: SessionCredentials = {};
	let active: ActiveCredentials | undefined;
	let loop: AbortController | undefined;
	let mode: string | undefined;
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

	async function restart(): Promise<void> {
		stop();
		active = resolveActive(credentials, env, await store.load());
		if (!active) return;
		await registerRemoteTools(pi, { client, getActive, state, warn });
		startLoop();
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
		/** Stops the loop and drops the credentials; registered tools stay and fail until the next login. */
		logout(): void {
			stop();
			active = undefined;
		},
		setMode(next: string | undefined): void {
			mode = next;
		},
	};
}

export function createBusExtension(options: BusExtensionOptions = {}): (pi: ExtensionAPI) => Promise<void> {
	return async pi => {
		const env = options.env ?? process.env;
		const store = createCredentialStore(env);
		const session = createSession(pi, env, store, options);

		pi.registerCommand("bus", {
			description: "Agent message bus: login <url> <token> | promote <leader-name> | status | logout | help",
			handler: async (args: string | string[] | undefined, ctx) => {
				session.setMode(ctx.mode);
				const deps = { store, client: session.client, env, session: session.credentials, restart: session.restart, stop: session.logout };
				const { message, level } = await runCommand(args, deps);
				ctx.ui.notify(message, level);
			},
		});

		pi.on("session_start", async (_event, ctx) => {
			session.setMode(ctx.mode);
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
