/**
 * Agent message bus: connects the session to an agent-message-bus server.
 *
 *   /bus login <url> <token>     store an admin, leader or member token
 *   /bus promote <leader-name>   mint and use a leader token (admin token required)
 *   /bus status | logout
 *
 * With agent credentials (stored, or env `AGENT_BUS_URL` + `AGENT_BUS_TOKEN`)
 * the server's `bus_*` tools are registered and the inbox is streamed into the
 * session: critical entries steer the running turn, others queue as follow-ups.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createClient, type FetchLike } from "./lib/client.ts";
import { runCommand } from "./lib/command.ts";
import { type ActiveCredentials, createCredentialStore, resolveActive } from "./lib/credentials.ts";
import { formatEntry, type InboxLoopOptions, runInboxLoop } from "./lib/inbox.ts";
import { registerRemoteTools } from "./lib/tools.ts";

export interface BusExtensionOptions {
	fetchFn?: FetchLike;
	env?: NodeJS.ProcessEnv;
	sleep?: InboxLoopOptions["sleep"];
}

function describe(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

export function createBusExtension(options: BusExtensionOptions = {}): (pi: ExtensionAPI) => Promise<void> {
	return async pi => {
		const env = options.env ?? process.env;
		const store = createCredentialStore(env);
		const client = createClient(options.fetchFn);
		const registration = { registered: false };
		let active: ActiveCredentials | undefined;
		let loop: AbortController | undefined;
		const getActive = () => active;

		function stop(): void {
			loop?.abort();
			loop = undefined;
		}

		async function restart(): Promise<void> {
			stop();
			active = resolveActive(env, await store.load());
			if (!active) return;
			await registerRemoteTools(pi, client, getActive, registration);
			const controller = new AbortController();
			loop = controller;
			void runInboxLoop({
				client,
				getActive,
				signal: controller.signal,
				sleep: options.sleep,
				onError: error => pi.logger.warn(`agent-bus: ${error.message}`),
				deliver: async entry => {
					pi.sendMessage(
						{ customType: "agent-bus", content: formatEntry(entry), display: true },
						{ deliverAs: entry.critical ? "steer" : "followUp", triggerTurn: true },
					);
				},
			}).catch(error => pi.logger.warn(`agent-bus: ${describe(error)}`));
		}

		pi.registerCommand("bus", {
			description: "Agent message bus: login <url> <token> | promote <leader-name> | status | logout",
			handler: async (args: string | string[] | undefined, ctx) => {
				const { message, level } = await runCommand(args, {
					store,
					client,
					env,
					restart,
					stop: () => {
						stop();
						active = undefined;
					},
				});
				ctx.ui.notify(message, level);
			},
		});

		pi.on("session_start", async () => {
			try {
				if (resolveActive(env, await store.load())) await restart();
			} catch (error) {
				pi.logger.warn(`agent-bus: ${describe(error)}`);
			}
		});
		pi.on("session_shutdown", () => stop());
	};
}

export default createBusExtension();
