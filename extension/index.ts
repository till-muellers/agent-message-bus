/**
 * Agent message bus: connects the session to an agent-message-bus server.
 *
 *   /bus login <url> <token>     store an admin, leader or member token
 *   /bus promote <leader-name>   mint and use a leader token (admin token required)
 *   /bus status | logout | help
 *
 * With agent credentials (stored, or env `AGENT_BUS_URL` + `AGENT_BUS_TOKEN`)
 * the server's `bus_*` tools are registered and the inbox is streamed into the
 * session (delivery policy: `lib/deliver.ts`).
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createClient, type FetchLike } from "./lib/client.ts";
import { runCommand } from "./lib/command.ts";
import { type ActiveCredentials, type CredentialStore, createCredentialStore, resolveActive } from "./lib/credentials.ts";
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

/** The session's bus connection: active credentials, registered tools and the inbox loop. */
function createSession(pi: ExtensionAPI, env: NodeJS.ProcessEnv, store: CredentialStore, options: BusExtensionOptions) {
	const client = createClient(options.fetchFn);
	const warn = (message: string) => pi.logger.warn(`agent-bus: ${message}`);
	const state = { registered: false };
	let active: ActiveCredentials | undefined;
	let loop: AbortController | undefined;
	let mode: string | undefined;
	const getActive = () => active;
	const deliver = createDeliver(pi, () => mode);

	function stop(): void {
		loop?.abort();
		loop = undefined;
	}

	async function restart(): Promise<void> {
		stop();
		active = resolveActive(env, await store.load());
		if (!active) return;
		await registerRemoteTools(pi, { client, getActive, state, warn });
		loop = new AbortController();
		void runInboxLoop({ client, getActive, signal: loop.signal, sleep: options.sleep, deliver, onError: error => warn(error.message) }).catch(error =>
			warn(describe(error)),
		);
	}

	return {
		client,
		warn,
		restart,
		stop,
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
				const deps = { store, client: session.client, env, restart: session.restart, stop: session.logout };
				const { message, level } = await runCommand(args, deps);
				ctx.ui.notify(message, level);
			},
		});

		pi.on("session_start", async (_event, ctx) => {
			session.setMode(ctx.mode);
			try {
				if (resolveActive(env, await store.load())) await session.restart();
			} catch (error) {
				session.warn(describe(error));
			}
		});
		pi.on("session_shutdown", () => session.stop());
	};
}

export default createBusExtension();
