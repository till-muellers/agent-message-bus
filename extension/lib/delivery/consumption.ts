/**
 * The entries a model request consumed: the agent-bus messages of the request carry the ids of the
 * entries they hold (`docs/design/arch/client.md` §3.3).
 */

/** The custom type of the messages the extension sends into the session. */
export const BUS_MESSAGE_TYPE = "agent-bus";

/** The details of an agent-bus message: the entries it carries. */
export interface BusMessageDetails {
	entryIds: number[];
}

/** A message of a model request, as far as the consumption looks at it. */
export interface RequestMessage {
	role?: string;
	customType?: string;
	details?: unknown;
}

/** The ids of the agent-bus messages of a request; the ids of other messages are ignored. */
export function consumedEntryIds(messages: readonly RequestMessage[]): number[] {
	const ids: number[] = [];
	for (const message of messages) {
		if (message.customType !== BUS_MESSAGE_TYPE) continue;
		const details = message.details as BusMessageDetails | undefined;
		for (const id of details?.entryIds ?? []) if (typeof id === "number") ids.push(id);
	}
	return ids;
}
