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

/** One fenced entry with the given (already escaped) body and hint. */
function fence(e: Entry, body: string | undefined, hint: string | undefined): string {
	const header = `[agent-bus] ${e.space} · ${where(e)} · from ${e.from}${e.critical ? " · CRITICAL" : ""}`;
	const lines = [openingTag(e), header, body, hint, "</agent-bus-entry>"];
	return lines.filter((line): line is string => line !== undefined).join("\n");
}

/** The envelope of one entry. */
export function envelope(e: Entry): string {
	return fence(e, e.body === undefined ? undefined : escapeBody(e.body), replyHint(e));
}

/** A post notice shows at most this many lines of a post… */
const NOTICE_LINES = 20;
/** …and at most this many characters (2 KiB). */
const NOTICE_CHARS = 2048;

/** The lines of a post a notice shows whole: at most NOTICE_LINES, together at most NOTICE_CHARS. */
function headOf(lines: readonly string[]): string[] {
	const head: string[] = [];
	let size = 0;
	for (const line of lines.slice(0, NOTICE_LINES)) {
		size += line.length + (head.length > 0 ? 1 : 0);
		if (size > NOTICE_CHARS) break;
		head.push(line);
	}
	return head;
}

/** The body of a post in a notice: whole when short, else its head and how many lines were left out. */
function noticeBody(e: Entry, body: string): string {
	const lines = body.split("\n");
	if (lines.length <= NOTICE_LINES && body.length <= NOTICE_CHARS) return escapeBody(body);
	const whole = headOf(lines);
	const shown = whole.length > 0 ? whole : [lines[0].slice(0, NOTICE_CHARS)];
	const left = lines.length - whole.length;
	const rest = `${left} ${left === 1 ? "line" : "lines"} left out; read the full thread with bus_read_thread {space: "${e.space}", threadId: ${e.threadId}}.`;
	return `${escapeBody(shown.join("\n"))}\n${rest}`;
}

/** The notice of the posts to one thread: every post fenced with its author, the hint once at the end. */
export function postNotice(entries: readonly Entry[]): string {
	const last = entries.length - 1;
	return entries
		.map((e, i) => fence(e, e.body === undefined ? undefined : noticeBody(e, e.body), i === last ? replyHint(e) : undefined))
		.join("\n");
}
