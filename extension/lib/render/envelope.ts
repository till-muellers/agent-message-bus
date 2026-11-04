/**
 * The model-facing text of an inbox entry (`docs/design/arch/client.md` §4.2,
 * `omp-rendering.feature` rule 3): one fenced `<agent-bus-entry …>` envelope with the
 * `[agent-bus]` header line, the escaped body and a hint how to answer. Sender, space and
 * criticality come from the entry's fields, never from its body.
 */
import type { Entry } from "../bus/entry.ts";
import { escapeBody, escapeTitle } from "./escape.ts";

function where(e: Entry): string {
	const title = escapeTitle(e.threadTitle ?? "");
	switch (e.type) {
		case "direct_message":
			return "direct";
		case "topic_message":
			return `topic #${e.topic}`;
		case "topic_closed":
			return `topic #${e.topic} closed`;
		case "thread_post":
			return `thread ${e.threadId} "${title}" on ${e.blackboard}`;
		case "thread_removed":
			return `thread ${e.threadId} "${title}" removed`;
		case "blackboard_removed":
			return `blackboard ${e.blackboard} removed`;
	}
}

/** How to answer the entry; lifecycle notices have nothing to answer. */
function replyHint(e: Entry): string | undefined {
	const direct = `bus_send_direct {space: "${e.space}", to: "${e.from}"}`;
	switch (e.type) {
		case "direct_message":
			return `Reply: ${direct}.`;
		case "topic_message":
			// Members cannot send to `all`: a broadcast is answered directly.
			if (e.topic === "all") return `Reply: ${direct}.`;
			return `Reply: bus_send_topic {space: "${e.space}", topic: "${e.topic}"}, or ${direct}.`;
		case "thread_post":
			return `Full thread: bus_read_thread {space: "${e.space}", threadId: ${e.threadId}}. Answer: bus_add_post {space: "${e.space}", threadId: ${e.threadId}}.`;
		default:
			return undefined;
	}
}

function openingTag(e: Entry): string {
	const critical = e.critical ? ' critical="true"' : "";
	return `<agent-bus-entry space="${e.space}" type="${e.type}" from="${e.from}" id="${e.id}"${critical}>`;
}

/** The envelope of one entry. */
export function envelope(e: Entry): string {
	const header = `[agent-bus] ${e.space} · ${where(e)} · from ${e.from}${e.critical ? " · CRITICAL" : ""}`;
	const body = e.body === undefined ? undefined : escapeBody(e.body);
	const lines = [openingTag(e), header, body, replyHint(e), "</agent-bus-entry>"];
	return lines.filter((line): line is string => line !== undefined).join("\n");
}
