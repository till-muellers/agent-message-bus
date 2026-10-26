/**
 * The context handler: omp fires it before every model request with the messages that are about to
 * be sent. The agent-bus messages among them name the entries the request consumed, and the
 * watermark acks them (`docs/design/arch/client.md` §3.3).
 */
import type { AckWatermark } from "../delivery/ack-watermark.ts";
import { consumedEntryIds, type RequestMessage } from "../delivery/consumption.ts";

export interface ContextHandlerDeps {
	/** The watermark of the acked entries. */
	watermark: AckWatermark;
	/** The acker: it tells the server how far the session consumed. */
	ack(upTo: number): Promise<void>;
}

/** The handler for omp's `context` event. */
export function createContextHandler(deps: ContextHandlerDeps): (event: { messages: readonly RequestMessage[] }) => void {
	return event => {
		const upTo = deps.watermark.consumed(consumedEntryIds(event.messages));
		if (upTo !== undefined) void deps.ack(upTo);
	};
}
