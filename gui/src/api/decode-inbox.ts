import { count, type Decoder, type Fields, fields, flag, listOf, optionalCount, optionalText, text } from "./decode.ts";
import type { Entry } from "./types.ts";

// Decoder of GET /web/api/inbox (contracts.md §10.2): a list of inbox entries.

/** The optional fields of an entry, each only when the server sent it. */
function details(o: Fields, what: string): Partial<Entry> {
	const found: Partial<Entry> = {
		topic: optionalText(o, "topic", what),
		threadId: optionalCount(o, "threadId", what),
		threadTitle: optionalText(o, "threadTitle", what),
		body: optionalText(o, "body", what),
	};
	return Object.fromEntries(Object.entries(found).filter(([, value]) => value !== undefined));
}

function entry(json: unknown, what: string): Entry {
	const o = fields(json, what);
	return {
		id: count(o, "id", what),
		space: text(o, "space", what),
		type: text(o, "type", what),
		from: text(o, "from", what),
		critical: flag(o, "critical", what),
		...details(o, what),
		createdAt: text(o, "createdAt", what),
	};
}

export const decodeEntries: Decoder<Entry[]> = (json) => listOf(json, "inbox", entry);
