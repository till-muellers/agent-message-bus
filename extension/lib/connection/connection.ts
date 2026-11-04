/**
 * The session's live bus connection: the active credentials, the registered `bus_*` tools and the inbox loop.
 * A main session publishes the credentials it acts with for its subagents (`root-identities.ts`); a subagent
 * acts only with what its root session published, registers no inbox tools and runs no inbox loop: its main
 * session owns the inbox.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { activateBusTools, deactivateBusTools, deactivateTools } from "./activation.ts";
import type { BusClient } from "../bus/client.ts";
import { createRootPublication, type RootIdentities, rootKeyOf } from "../session/root-identities.ts";
import type { SessionCredentials } from "../session/session-credentials.ts";
import { type ActiveCredentials, type CredentialStore } from "../session/stored-credentials.ts";
import { createAckWatermark } from "../delivery/ack-watermark.ts";
import { createAcker } from "../delivery/acker.ts";
import { type Consume, createConsumer } from "../delivery/consumer.ts";
import { NOT_LOGGED_IN } from "../bus/errors.ts";
import { phaseOf } from "../delivery/delivery-class.ts";
import { createDispatcher, type Dispatcher } from "../delivery/dispatcher.ts";
import type { InboxLoopOptions } from "../delivery/inbox-loop.ts";
import { createInboxRunner } from "../delivery/inbox-runner.ts";
import type { SessionIdentity } from "../session/session-identity.ts";
import { createUnreadPosts } from "../render/status-line.ts";
import { registerRemoteTools } from "../tools/remote-tools.ts";
import { WAIT_TOOL, type WaitToolDeps } from "../tools/wait-tool.ts";
import type { ServerGate } from "./server-gate.ts";

/** What the omp context says about the current session; updated on every event and command. */
export interface SessionView {
	mode?: string;
	sub: boolean;
	/** The id of the agent that runs the session (`ctx.agent.id`); the subagent's id in a subagent session. */
	agentId?: string;
	/** The session's file (`ctx.sessionManager.getSessionFile()`): the root key of a main session, the way to it for a subagent. */
	sessionFile?: string;
	/** Whether the agent is idle right now (bound to the latest omp context). */
	isIdle(): boolean;
	/** Sets or (with `undefined`) removes a status line entry (bound to the latest omp context). */
	setStatus(key: string, text: string | undefined): void;
}

export interface ConnectionDeps {
	pi: ExtensionAPI;
	client: BusClient;
	store: CredentialStore;
	credentials: Pick<SessionCredentials, "resolve">;
	identity: Pick<SessionIdentity, "load">;
	/** The identities root sessions publish for their subagents; shared by every session of the process. */
	roots: RootIdentities;
	view: SessionView;
	/** Keeps the extension off a server that is too old; checked before the tools are activated. */
	gate: ServerGate;
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
	/** Tools this extension registers itself (`bus_spawn_worker`); they are switched on and off with the server tools. */
	localTools: readonly string[];
	/** Awaited after a successful membership tool (`tools/membership-refresh.ts`). */
	membershipChanged?: () => Promise<void>;
}

export interface Connection {
	getActive(): ActiveCredentials | undefined;
	/** The dispatcher of this connection; it knows what it handed over. */
	dispatcher: Dispatcher;
	/** Marks entries consumed; the ack follows the watermark. */
	consume: Consume;
	/** Re-resolves the credentials, registers the server tools once, restarts the inbox loop and switches the tools on or off to match. */
	restart(): Promise<void>;
	/** Stops the loop, drops the credentials and deactivates every bus tool; registered tools stay and fail until the next login. */
	logout(): Promise<void>;
	/** Session end: stops the loop and withdraws the identity published for the subagents. */
	stop(): void;
}

/** The server's `bus_wait` for a session without its own stream; it acks nothing. */
function serverWait(client: BusClient, getActive: () => ActiveCredentials | undefined): WaitToolDeps["forward"] {
	return async (args, timeoutMs, signal) => {
		const current = getActive();
		if (!current) throw new Error(NOT_LOGGED_IN);
		return client.callTool(current.url, current.token, WAIT_TOOL, { ...args }, { timeoutMs, signal });
	};
}

export function createConnection(deps: ConnectionDeps): Connection {
	const { pi, client, view, warn, localTools } = deps;
	const tools = { registered: false, names: [] as string[] };
	const publication = createRootPublication(deps.roots);
	let active: ActiveCredentials | undefined;
	// A subagent reads its root session's identity at every call, never the store or the env, so a later
	// promote or logout of the main session reaches its next call.
	const getActive = () => (view.sub ? deps.roots.lookup(view.sessionFile) : active);
	let toldNoIdentity = false;
	const watermark = createAckWatermark();
	const consume = createConsumer({ watermark, ack: createAcker({ client, getActive, warn }) });
	const unread = createUnreadPosts((key, text) => view.setStatus(key, text));
	const dispatcher = createDispatcher({
		phase: () => phaseOf(view.mode, view.isIdle()),
		watermark,
		handOver: (message, mechanism) => {
			pi.sendMessage(message, mechanism);
			unread.add(message.details.entries);
		},
		warn,
	});
	const inbox = createInboxRunner({ client, getActive, warn, sleep: deps.sleep, dispatcher });
	const wait: WaitToolDeps = {
		dispatcher: () => (inbox.isOpen() ? dispatcher : undefined),
		forward: serverWait(client, getActive),
		consumed: consume,
	};
	const ours = () => [...tools.names, ...localTools];

	/** A main session resolves its own credentials and publishes them; a subagent takes its root session's. */
	async function resolve(): Promise<ActiveCredentials | undefined> {
		if (view.sub) return getActive();
		await deps.identity.load();
		active = deps.credentials.resolve(await deps.store.load());
		publication.set(rootKeyOf(view.sessionFile), active);
		return active;
	}

	/** Registers the session's tools unless the server is too old; false when it is. */
	async function connect(creds: ActiveCredentials): Promise<boolean> {
		const verdict = await deps.gate.check(creds.url);
		if (verdict !== "ok") {
			warn(verdict.refused);
			return false;
		}
		const kind = view.sub ? "sub" : "main";
		const hooks = { threadRead: unread.read, membershipChanged: deps.membershipChanged };
		await registerRemoteTools(pi, { client, getActive, state: tools, warn, kind, agentId: view.agentId, wait, ...hooks });
		return true;
	}

	/** A subagent without its root session's identity gets no bus tools: one line, then silence. */
	function noIdentity(): void {
		if (!view.sub || toldNoIdentity) return;
		toldNoIdentity = true;
		warn("no bus tools for the subagent: its main session has no bus identity");
	}

	return {
		getActive,
		dispatcher,
		consume,
		stop() {
			inbox.stop();
			publication.clear();
		},
		async restart() {
			inbox.stop();
			// The posts were handed to the session or identity that is going away.
			unread.clear();
			const creds = await resolve();
			if (!creds) noIdentity();
			if (!creds || !(await connect(creds))) return deactivateTools(pi, ours());
			if (!view.sub) inbox.start();
			await activateBusTools(pi, ours());
		},
		async logout() {
			inbox.stop();
			active = undefined;
			publication.clear();
			unread.clear();
			await deactivateBusTools(pi);
		},
	};
}
