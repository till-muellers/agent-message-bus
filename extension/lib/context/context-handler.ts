/**
 * The context handler: omp fires it before every model request with the messages that are about to
 * be sent. The agent-bus messages among them name the entries the request consumed
 * (`docs/design/arch/client.md` §3.3).
 */
import type { Consume } from "../delivery/consumer.ts";
import { consumedEntryIds, type RequestMessage } from "../delivery/consumption.ts";

/** The handler for omp's `context` event. */
export function createContextHandler(consume: Consume): (event: { messages: readonly RequestMessage[] }) => void {
	return event => consume(consumedEntryIds(event.messages));
}
