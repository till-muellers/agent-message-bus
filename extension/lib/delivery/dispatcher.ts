/**
 * The dispatcher: it receives an inbox entry and hands it over to omp with the mechanism its
 * delivery class and the session's phase ask for (`docs/design/arch/client.md` §3.2).
 *
 * The message carries the id of the entry, so the request that consumes it acks it. A reconnect
 * replays the entries that were not acked; the ones this connection already handed over are skipped.
 */
import type { Entry } from "../bus/entry.ts";
import { parseDeliveryClass } from "../bus/entry.ts";
import { formatEntry } from "../render/envelope.ts";
import type { AckWatermark } from "./ack-watermark.ts";
import type { BusMessageDetails } from "./consumption.ts";
import { mechanismFor, type Mechanism, type SessionPhase } from "./delivery-class.ts";

export interface Dispatcher {
	/** Receives an entry of the stream and hands it over to the session. */
	receive(entry: Entry): void;
}

export interface DispatcherDeps {
	/** The phase of the session right now. */
	phase(): SessionPhase;
	/** The watermark of the acked entries. */
	watermark: AckWatermark;
	/** Hands the formatted entry over to omp with the mechanism. */
	handOver(content: string, mechanism: Mechanism, details: BusMessageDetails): void;
}

/** The dispatcher of one connection: it maps every entry to the mechanism its class asks for. */
export function createDispatcher(deps: DispatcherDeps): Dispatcher {
	const handedOver = new Set<number>();
	return {
		receive(entry) {
			deps.watermark.received(entry.id);
			// A reconnect replays the entries that were not acked; this connection already handed them over.
			if (handedOver.has(entry.id)) return;
			handedOver.add(entry.id);
			deps.handOver(formatEntry(entry), mechanismFor(parseDeliveryClass(entry.class), deps.phase()), { entryIds: [entry.id] });
		},
	};
}
