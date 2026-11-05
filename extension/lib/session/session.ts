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
import { type Connection, createConnection } from "../connection/connection.ts";
import type { SessionView } from "../connection/session-view.ts";
import type { ServerGate } from "../connection/server-gate.ts";
import { type BusContext, createBusContext } from "../context/bus-context.ts";
import { createBusEvents } from "../delivery/bus-events.ts";
import type { SessionCredentials } from "./session-credentials.ts";
import { createSessionCredentials } from "./session-credentials.ts";
import { createSessionIdentity, type IdentityContext, type SessionChange, type SessionIdentity } from "./session-identity.ts";
import type { RootIdentities } from "./root-identities.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import type { CredentialStore } from "./stored-credentials.ts";
import type { BusEnv } from "./env.ts";

/** The parts of the omp context a session reads: mode, agent kind, idleness and the session file. */
export interface SessionContext extends IdentityContext {
	mode?: string;
	/** The agent the session runs: its kind and its registry id. */
	agent?: { kind?: string; id?: string };
	/** Whether the agent is idle right now. */
	isIdle(): boolean;
	/** omp's UI: the status line. */
	ui?: { setStatus(key: string, text: string | undefined): void };
}

/** The session's bus state: its credentials, accepted identity and connection. */
export interface Session extends Connection {
	client: BusClient;
	credentials: SessionCredentials;
	/** The stored login. */
	store: CredentialStore;
	/** Keeps the extension off a server that is too old. */
	gate: ServerGate;
	warn(message: string): void;
	isSub(): boolean;
	clearAccepted(): Promise<void>;
	accept(url: string, code: string): Promise<Joined>;
	/** After an accept: reconnects as the member, which switches the tools on, and sends the invite's first prompt. */
	join(joined: Joined): Promise<CommandResult>;
	/** Session end: stops the loop and forgets the promoted leader token. */
	shutdown(): void;
	/** The agent-bus-context block for the next model request (cached data only). */
	contextBlock(): string | undefined;
	/** The bus agent the session acts as (cached); undefined while unknown. */
	self(): string | undefined;
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
	/** The identities root sessions publish for their subagents; one registry per extension factory. */
	roots: RootIdentities;
	/** The sleep the inbox loop waits with. */
	sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
	/** How long a one-shot run waits for the inbox backlog after an invite; 5 s by default. */
	backlogTimeoutMs?: number;
}

/** How long a one-shot run waits for the inbox backlog after it redeemed an invite. */
const BACKLOG_TIMEOUT_MS = 5_000;

/** The highest id in the session's inbox; undefined when the inbox is empty or unreadable. */
async function highestInboxId(client: BusClient, active: ActiveCredentials | undefined): Promise<number | undefined> {
	if (!active) return undefined;
	try {
		const result = await client.callTool(active.url, active.token, "bus_inbox_read", {});
		if (!Array.isArray(result)) return undefined;
		const ids = result
			.map(entry => (entry as { id?: unknown }).id)
			.filter((id): id is number => typeof id === "number");
		return ids.length ? Math.max(...ids) : undefined;
	} catch {
		return undefined;
	}
}

/** A one-shot run's wait for the inbox backlog after it redeemed an invite. */
interface BacklogWait {
	client: BusClient;
	connection: Pick<Connection, "getActive" | "ledger">;
	view: SessionView;
	warn(message: string): void;
	timeoutMs: number;
}

/**
 * A one-shot run starts its own prompt right after the session start, so the entries of the redeemed invite must be
 * queued before it. The wait ends when the stream handed them over, or after the timeout.
 */
async function waitForBacklog({ client, connection, view, warn, timeoutMs }: BacklogWait): Promise<void> {
	if (view.mode !== "print" && view.mode !== "json") return;
	const maxId = await highestInboxId(client, connection.getActive());
	if (maxId === undefined) return;
	const delivered = await connection.ledger().handedUpTo(maxId, timeoutMs);
	if (!delivered) warn(`inbox backlog not delivered within ${timeoutMs / 1000} s; continuing`);
}

/** The latest omp context, as the connection reads it. */
function bindView(view: SessionView, ctx: SessionContext): void {
	view.mode = ctx.mode;
	view.sub = ctx.agent?.kind === "sub";
	view.agentId = ctx.agent?.id;
	view.sessionFile = ctx.sessionManager?.getSessionFile();
	view.isIdle = () => ctx.isIdle();
	view.setStatus = (key, text) => ctx.ui?.setStatus(key, text);
}

/** What the session is made of besides its deps. */
interface SessionParts {
	credentials: SessionCredentials;
	identity: SessionIdentity;
	view: SessionView;
	warn(message: string): void;
}

/** Redeeming an invite: accept posts the code, join reconnects as the member and, in a one-shot run, waits for the backlog. */
function inviteOps(deps: SessionDeps, parts: SessionParts, connection: Connection): Pick<Session, "accept" | "join" | "clearAccepted"> {
	const { client, gate } = deps;
	const { credentials, identity, view, warn } = parts;
	const timeoutMs = deps.backlogTimeoutMs ?? BACKLOG_TIMEOUT_MS;
	return {
		accept: (url, code) => acceptInvite({ client, session: credentials, identity, gate }, url, code),
		async join(joined) {
			await connection.restart();
			await waitForBacklog({ client, connection, view, warn, timeoutMs });
			return joined.result;
		},
		clearAccepted: () => identity.clear(),
	};
}

/** The connection and the context block it keeps fresh: re-read on every connected restart, dropped when not connected. */
function connect(deps: SessionDeps, { credentials, identity, view, warn }: SessionParts): { connection: Connection; busContext: BusContext; self(): string | undefined } {
	const hooks = {
		membershipChanged: () => busContext.refreshIdentity(),
		connected: async (connected: boolean) => (connected ? busContext.refresh() : busContext.clear()),
		self: () => busContext.who().agent,
	};
	const events = createBusEvents(deps.pi.events, () => busContext.who(), warn);
	const connection = createConnection({ ...deps, credentials, identity, view, warn, events, ...hooks });
	const busContext = createBusContext({ client: deps.client, getActive: connection.getActive, warn, isSub: () => view.sub, agentId: () => view.agentId });
	return { connection, busContext, self: hooks.self };
}

/** Builds the session of this extension. */
export function createSession(deps: SessionDeps): Session {
	const warn = (message: string) => deps.pi.logger.warn(`agent-bus: ${message}`);
	const credentials = createSessionCredentials(deps.busEnv.credentials);
	const identity = createSessionIdentity(deps.dir, credentials);
	const view: SessionView = { sub: false, isIdle: () => true, setStatus: () => {} };
	const parts = { credentials, identity, view, warn };
	const { connection, busContext, self } = connect(deps, parts);
	return {
		...connection,
		...inviteOps(deps, parts, connection),
		async logout() {
			await connection.logout();
			busContext.clear();
		},
		contextBlock: busContext.block,
		self,
		client: deps.client,
		credentials,
		store: deps.store,
		gate: deps.gate,
		warn,
		isSub: () => view.sub,
		shutdown() {
			connection.stop();
			credentials.ended();
		},
		setContext(ctx, change) {
			bindView(view, ctx);
			return identity.bind(ctx, change);
		},
	};
}
