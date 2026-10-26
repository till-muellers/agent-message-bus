/**
 * The session of the extension: its credentials, its accepted identity and its
 * connection to the server (client.md §2.2).
 *
 * `index.ts` wires the session up; everything the session *is* lives here.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { BusClient } from "../bus/client.ts";
import type { CommandResult } from "../shared/command.ts";
import { acceptInvite, type Joined } from "../commands/accept.ts";
import { type Connection, createConnection, type SessionView } from "../connection/connection.ts";
import type { ServerGate } from "../connection/server-gate.ts";
import type { SessionCredentials } from "./session-credentials.ts";
import { createSessionCredentials } from "./session-credentials.ts";
import { createSessionIdentity, type IdentityContext, type SessionChange } from "./session-identity.ts";
import type { CredentialStore } from "./stored-credentials.ts";
import type { BusEnv } from "./env.ts";

/** The parts of the omp context a session reads: mode, agent kind, idleness and the session file. */
export interface SessionContext extends IdentityContext {
	mode?: string;
	/** Whether the agent is idle right now. */
	isIdle(): boolean;
}

/** The session's bus state: its credentials, accepted identity and connection. */
export interface Session extends Connection {
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

/** What the session needs to be built. */
export interface SessionDeps {
	pi: ExtensionAPI;
	store: CredentialStore;
	busEnv: BusEnv;
	/** The directory holding the per-session files. */
	dir: string;
	/** The client the session talks to the server with. */
	client: BusClient;
	/** The tools this extension registers itself (`bus_spawn_worker`). */
	localTools: readonly string[];
	/** Keeps the extension off a server that is too old. */
	gate: ServerGate;
	/** The sleep the inbox loop waits with. */
	sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
}

/** Builds the session of this extension. */
export function createSession(deps: SessionDeps): Session {
	const { pi, store, busEnv, dir, client, localTools, gate } = deps;
	const warn = (message: string) => pi.logger.warn(`agent-bus: ${message}`);
	const credentials = createSessionCredentials(busEnv.credentials);
	const identity = createSessionIdentity(dir, credentials);
	const view: SessionView = { sub: false, isIdle: () => true };
	const connection = createConnection({ pi, client, store, credentials, identity, view, gate, warn, sleep: deps.sleep, localTools });
	return {
		...connection,
		client,
		credentials,
		warn,
		isSub: () => view.sub,
		clearAccepted: () => identity.clear(),
		accept: (url, code) => acceptInvite({ client, session: credentials, identity, gate }, url, code),
		async join(joined) {
			await connection.restart();
			if (joined.task) pi.sendUserMessage(joined.task);
			return joined.result;
		},
		shutdown() {
			connection.stop();
			credentials.ended();
		},
		setContext(ctx, change) {
			view.mode = ctx.mode;
			view.sub = ctx.agent?.kind === "sub";
			view.isIdle = () => ctx.isIdle();
			return identity.bind(ctx, change);
		},
	};
}
