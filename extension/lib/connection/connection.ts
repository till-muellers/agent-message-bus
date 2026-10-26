/**
 * The session's live bus connection: the active credentials, the registered `bus_*` tools and the inbox loop.
 * Subagents register no inbox tools and run no inbox loop: their parent session owns the inbox.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { activateBusTools, deactivateBusTools, deactivateTools } from "./activation.ts";
import type { BusClient } from "../bus/client.ts";
import type { SessionCredentials } from "../session/session-credentials.ts";
import { type ActiveCredentials, type CredentialStore } from "../session/stored-credentials.ts";
import type { AckWatermark } from "../delivery/ack-watermark.ts";
import { phaseOf } from "../delivery/delivery-class.ts";
import { createDispatcher } from "../delivery/dispatcher.ts";
import type { InboxLoopOptions } from "../delivery/inbox-loop.ts";
import { createInboxRunner } from "../delivery/inbox-runner.ts";
import type { SessionIdentity } from "../session/session-identity.ts";
import { INBOX_TOOLS, registerRemoteTools } from "../tools/remote-tools.ts";
import type { ServerGate } from "./server-gate.ts";

/** What the omp context says about the current session; updated on every event and command. */
export interface SessionView {
	mode?: string;
	sub: boolean;
	/** Whether the agent is idle right now (bound to the latest omp context). */
	isIdle(): boolean;
}

export interface ConnectionDeps {
	pi: ExtensionAPI;
	client: BusClient;
	store: CredentialStore;
	credentials: Pick<SessionCredentials, "resolve">;
	identity: Pick<SessionIdentity, "load">;
	view: SessionView;
	/** Keeps the extension off a server that is too old; checked before the tools are activated. */
	gate: ServerGate;
	/** The watermark of the acked entries; the dispatcher tells it what arrived. */
	watermark: AckWatermark;
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
	/** Tools this extension registers itself (`bus_spawn_worker`); they are switched on and off with the server tools. */
	localTools: readonly string[];
}

export interface Connection {
	getActive(): ActiveCredentials | undefined;
	/** Re-resolves the credentials, registers the server tools once, restarts the inbox loop and switches the tools on or off to match. */
	restart(): Promise<void>;
	/** Stops the loop, drops the credentials and deactivates every bus tool; registered tools stay and fail until the next login. */
	logout(): Promise<void>;
	stop(): void;
}

export function createConnection({ pi, client, store, credentials, identity, view, gate, watermark, warn, sleep, localTools }: ConnectionDeps): Connection {
	const tools = { registered: false, names: [] as string[] };
	let active: ActiveCredentials | undefined;
	const getActive = () => active;
	const dispatcher = createDispatcher({
		phase: () => phaseOf(view.mode, view.isIdle()),
		watermark,
		handOver: (content, mechanism, details) => pi.sendMessage({ customType: "agent-bus", content, display: true, details }, mechanism),
	});
	const inbox = createInboxRunner({ client, getActive, warn, sleep, dispatcher });
	const ours = () => [...tools.names, ...localTools];
	return {
		getActive,
		stop: inbox.stop,
		async restart() {
			inbox.stop();
			await identity.load();
			active = credentials.resolve(await store.load());
			if (active) {
				// A server that is too old gets no tools and no stream.
				const verdict = await gate.check(active.url);
				if (verdict !== "ok") {
					warn(verdict.refused);
					await deactivateTools(pi, ours());
					return;
				}
				await registerRemoteTools(pi, { client, getActive, state: tools, warn, exclude: view.sub ? INBOX_TOOLS : undefined });
			}
			if (active && !view.sub) inbox.start();
			if (active) await activateBusTools(pi, ours());
			else await deactivateTools(pi, ours());
		},
		async logout() {
			inbox.stop();
			active = undefined;
			await deactivateBusTools(pi);
		},
	};
}
