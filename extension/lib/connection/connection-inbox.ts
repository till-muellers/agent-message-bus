/**
 * The inbox side of the connection: the inbox ledger of the current connection start, the stream that fills it,
 * the status line of the handed-over posts and what the local `bus_wait` waits with (`docs/design/arch/client.md` §3).
 * Each connection start begins with a new ledger: what the last start handed over or left pending is forgotten.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { BusClient } from "../bus/client.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import { NOT_LOGGED_IN } from "../bus/errors.ts";
import { createAcker } from "../delivery/acker.ts";
import type { BusEvents } from "../delivery/bus-events.ts";
import { phaseOf } from "../delivery/delivery-class.ts";
import { createInboxLedger, type InboxLedger } from "../delivery/inbox-ledger.ts";
import type { InboxLoopOptions } from "../delivery/inbox-loop.ts";
import { createInboxRunner } from "../delivery/inbox-runner.ts";
import { createUnreadPosts, type UnreadPosts } from "../render/status-line.ts";
import { WAIT_TOOL, type WaitToolDeps } from "../tools/wait-tool.ts";
import type { SessionView } from "./session-view.ts";

export interface ConnectionInboxDeps {
	pi: Pick<ExtensionAPI, "sendMessage">;
	client: BusClient;
	view: SessionView;
	warn(message: string): void;
	sleep?: InboxLoopOptions["sleep"];
	/** Where handed-over entries and connection changes are published for other extensions. */
	events?: BusEvents;
}

export interface ConnectionInbox {
	/** The inbox ledger of the current connection start. */
	ledger(): InboxLedger;
	/** Marks entries consumed in the current start's ledger. */
	consume(ids: readonly number[]): void;
	/** What the local `bus_wait` waits with. */
	wait: WaitToolDeps;
	/** Told when `bus_read_thread` read a thread: its posts leave the status line. */
	threadRead(space: string, threadId: number): void;
	/** A new connection start: stops the stream, begins a new ledger and clears the status line. */
	startOver(): void;
	/** Streams the inbox into the current ledger. */
	open(): void;
	/** Stops the stream; the held entries of the ledger are dropped unhanded. */
	stop(): void;
	/** Clears the status line: the posts were handed to an identity that is going away. */
	forgetPosts(): void;
}

/** The server's `bus_wait` for a session without its own stream; it acks nothing. */
function serverWait(client: BusClient, getActive: () => ActiveCredentials | undefined): WaitToolDeps["forward"] {
	return async (args, timeoutMs, signal) => {
		const current = getActive();
		if (!current) throw new Error(NOT_LOGGED_IN);
		return client.callTool(current.url, current.token, WAIT_TOOL, { ...args }, { timeoutMs, signal });
	};
}

/** Builds the ledger of one connection start: it hands entries to omp, the status line and the bus events. */
function ledgerFactory(deps: ConnectionInboxDeps, getActive: () => ActiveCredentials | undefined, unread: UnreadPosts): () => InboxLedger {
	const { pi, client, view, warn } = deps;
	const ack = createAcker({ client, getActive, warn });
	return () =>
		createInboxLedger({
			phase: () => phaseOf(view.mode, view.isIdle()),
			handOver: (message, mechanism) => {
				pi.sendMessage(message, mechanism);
				unread.add(message.details.entries);
				for (const entry of message.details.entries) deps.events?.entry(entry, mechanism.deliverAs);
			},
			warn,
			ack,
		});
}

export function createConnectionInbox(deps: ConnectionInboxDeps, getActive: () => ActiveCredentials | undefined): ConnectionInbox {
	const { client, view, warn } = deps;
	const unread = createUnreadPosts((key, text) => view.setStatus(key, text));
	const startLedger = ledgerFactory(deps, getActive, unread);
	let ledger = startLedger();
	const runner = createInboxRunner({ client, getActive, warn, sleep: deps.sleep, onState: deps.events?.connection });
	return {
		ledger: () => ledger,
		consume: ids => ledger.consumed(ids),
		wait: {
			source: () => (runner.isOpen() ? ledger : undefined),
			claims: { claimFromServer: entries => ledger.claimFromServer(entries) },
			forward: serverWait(client, getActive),
		},
		threadRead: unread.read,
		startOver() {
			runner.stop();
			ledger = startLedger();
			unread.clear();
		},
		open: () => runner.start(ledger),
		stop: runner.stop,
		forgetPosts: unread.clear,
	};
}
