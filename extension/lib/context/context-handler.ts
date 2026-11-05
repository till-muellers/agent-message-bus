/**
 * The context handler: omp fires it before every model request with the messages that are about to
 * be sent. The agent-bus messages among them name the entries the request consumed
 * (`docs/design/arch/client.md` §3.3); then the agent-bus-context block goes in front of them (§5).
 * The handler reads only cached data, so it never waits for the network.
 */
import { consumedEntryIds, type RequestMessage } from "../delivery/consumption.ts";

/** The custom type of the block; omp sends it to the model as a developer message and never stores it. */
export const CONTEXT_MESSAGE_TYPE = "agent-bus-context";

export interface ContextMessage {
	role: "custom";
	customType: typeof CONTEXT_MESSAGE_TYPE;
	content: string;
	display: false;
	timestamp: number;
}

export interface ContextHandlerDeps {
	/** Marks the entries the request carried consumed (the current start's inbox ledger). */
	consume(ids: readonly number[]): void;
	/** The block text right now; undefined when the session has no bus identity. */
	block(): string | undefined;
}

/** The handler for omp's `context` event. */
export function createContextHandler(
	deps: ContextHandlerDeps,
): <M extends RequestMessage>(event: { messages: readonly M[] }) => { messages: (M | ContextMessage)[] } | undefined {
	// The same text keeps the same message object, so the request prefix stays stable for the prompt cache.
	let last: ContextMessage | undefined;
	return event => {
		deps.consume(consumedEntryIds(event.messages));
		const content = deps.block();
		if (content === undefined) return undefined;
		if (last?.content !== content) last = { role: "custom", customType: CONTEXT_MESSAGE_TYPE, content, display: false, timestamp: Date.now() };
		return { messages: [last, ...event.messages] };
	};
}
