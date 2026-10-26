/**
 * The dispatcher: it receives an inbox entry and hands it over to omp with the mechanism its
 * delivery class and the session's phase ask for (`docs/design/arch/client.md` §3.2).
 *
 * Holding entries for coalescing and the ack watermark come later; this is the plain path.
 */
import type { Entry } from "../bus/entry.ts";
import { parseDeliveryClass } from "../bus/entry.ts";
import { formatEntry } from "../render/envelope.ts";
import { mechanismFor, type Mechanism, type SessionPhase } from "./delivery-class.ts";

export interface Dispatcher {
	/** Receives an entry of the stream and hands it over to the session. */
	receive(entry: Entry): void;
}

export interface DispatcherDeps {
	/** The phase of the session right now. */
	phase(): SessionPhase;
	/** Hands the formatted entry over to omp with the mechanism. */
	handOver(content: string, mechanism: Mechanism): void;
}

/** The dispatcher of one connection: it maps every entry to the mechanism its class asks for. */
export function createDispatcher(deps: DispatcherDeps): Dispatcher {
	return {
		receive(entry) {
			deps.handOver(formatEntry(entry), mechanismFor(parseDeliveryClass(entry.class), deps.phase()));
		},
	};
}
