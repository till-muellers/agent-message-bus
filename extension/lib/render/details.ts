/**
 * The agent-bus session message (`docs/design/arch/client.md` §4.1): the envelope is the
 * model-facing content; the details keep the entry itself, for the ack (`entryIds`) and for
 * the renderers.
 */
import type { Entry } from "../bus/entry.ts";
import { envelope } from "./envelope.ts";

/** The custom type of the messages the extension sends into the session. */
export const BUS_MESSAGE_TYPE = "agent-bus";

/** The details of an agent-bus message: the entries it carries. */
export interface BusMessageDetails {
	/** The ack key: the ids of the entries the message carries. */
	entryIds: number[];
	entries: Entry[];
}

/** A custom message of the extension, as `pi.sendMessage` takes it. */
export interface BusMessage {
	customType: typeof BUS_MESSAGE_TYPE;
	content: string;
	display: true;
	details: BusMessageDetails;
}

/** The entry's known fields; anything else the wire carries stays out of the session. */
function entryFields(e: Entry): Entry {
	const { id, space, type, from, critical, class: cls, createdAt, body, topic, blackboard, threadId, threadTitle } = e;
	return { id, space, type, from, critical, class: cls, createdAt, body, topic, blackboard, threadId, threadTitle };
}

/** The session message of one entry. */
export function busMessage(entry: Entry): BusMessage {
	return {
		customType: BUS_MESSAGE_TYPE,
		content: envelope(entry),
		display: true,
		details: { entryIds: [entry.id], entries: [entryFields(entry)] },
	};
}
