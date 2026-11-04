/**
 * Bus activity on omp's extension event bus (`pi.events`, `docs/design/arch/client.md` §3.6, `omp-events.feature`):
 * every entry handed over to the session and every connection change, for other extensions of the session to
 * observe. A listener that throws is logged; it never stops delivery. The same bus carries the handshake that
 * keeps a second loaded copy of this extension off.
 */
import type { Entry } from "../bus/entry.ts";
import { errorMessage } from "../bus/errors.ts";
import type { Mechanism } from "./delivery-class.ts";

export const ENTRY_CHANNEL = "agent-bus:entry";
export const CONNECTION_CHANNEL = "agent-bus:connection";
export const INSTANCE_CHANNEL = "agent-bus:instance";

/** The parts of omp's `EventBus` the extension uses. */
export interface EventChannels {
	emit(channel: string, data: unknown): void;
	on(channel: string, handler: (data: unknown) => void): () => void;
}

export type ConnectionState = "connected" | "conflict" | "retrying" | "logged-out";

/** Who the session is on the bus, as far as it knows. */
export interface BusWho {
	agent?: string;
	space?: string;
}

export interface BusEvents {
	/** An entry was handed over to the session with this delivery mode. */
	entry(entry: Entry, deliverAs: Mechanism["deliverAs"]): void;
	connection(state: ConnectionState): void;
}

export function createBusEvents(events: EventChannels, who: () => BusWho, warn: (message: string) => void): BusEvents {
	const publish = (channel: string, data: unknown): void => {
		try {
			events.emit(channel, data);
		} catch (error) {
			warn(`${channel} listener failed: ${errorMessage(error)}`);
		}
	};
	return {
		entry: (entry, deliverAs) => publish(ENTRY_CHANNEL, { entry, deliverAs }),
		connection: state => publish(CONNECTION_CHANNEL, { state, ...who() }),
	};
}

export interface Instance {
	/** Whether another copy announced itself on this event bus first, so this one must stay off. */
	deferred(): boolean;
}

interface Announcement {
	instanceId: string;
	/** `hello`: a copy just loaded; `present`: an active copy answers it. */
	kind: "hello" | "present";
}

function isAnnouncement(data: unknown): data is Announcement {
	const it = data as Partial<Announcement> | null;
	return typeof it?.instanceId === "string" && (it.kind === "hello" || it.kind === "present");
}

/**
 * The two-copy handshake: a copy listens, then says hello. An active copy that hears another's hello answers
 * `present`, and the copy that hears `present` defers. The bus fans out synchronously, so the answer arrives
 * before the hello returns.
 */
export function announceInstance(events: EventChannels): Instance {
	const instanceId = crypto.randomUUID();
	let deferred = false;
	events.on(INSTANCE_CHANNEL, data => {
		if (!isAnnouncement(data) || data.instanceId === instanceId || deferred) return;
		if (data.kind === "present") deferred = true;
		else events.emit(INSTANCE_CHANNEL, { instanceId, kind: "present" } satisfies Announcement);
	});
	events.emit(INSTANCE_CHANNEL, { instanceId, kind: "hello" } satisfies Announcement);
	return { deferred: () => deferred };
}
