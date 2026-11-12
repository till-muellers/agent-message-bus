/**
 * The model-facing text of an inbox entry (`docs/design/arch/client.md` §4.2,
 * `omp-rendering.feature` rule 3): one fenced `<agent-bus-entry …>` envelope with the
 * header line, the escaped body and a hint how to answer. Sender, space and
 * criticality come from the entry's fields, never from its body.
 */
import type { Entry } from "../bus/entry.ts";
import { labelOf } from "./entry-label.ts";
import { escapeBody, escapeTitle } from "./escape.ts";
import { isoMillis } from "./iso-millis.ts";

/** The post number of a thread post, when the server sends it. */
function postLabel(e: Entry): string {
	return e.postId === undefined ? "" : ` · post ${e.postId}`;
}

function where(e: Entry): string {
	const { about, ended } = labelOf(e);
	const title = escapeTitle(e.threadTitle ?? "");
	switch (about) {
		case "direct":
			return "direct";
		case "topic":
			return ended ? `topic #${e.topic} closed` : `topic #${e.topic}`;
		case "thread":
			return ended ? `thread ${e.threadId} "${title}" removed` : `thread ${e.threadId} "${title}" on ${e.blackboard}${postLabel(e)}`;
		case "blackboard":
			return `blackboard ${e.blackboard} removed`;
		case "other":
			return escapeTitle(e.type);
	}
}

/** How to answer the entry; lifecycle notices and entries of an unknown type have nothing to answer. */
function replyHint(e: Entry): string | undefined {
	const { about, ended } = labelOf(e);
	if (ended) return undefined;
	const direct = `bus_send_direct {space: "${e.space}", to: "${e.from}"}`;
	const topic = `bus_send_topic {space: "${e.space}", topic: "${e.topic}"}`;
	const notJoined = `${e.from} has not joined ${e.space}, so a direct reply cannot reach it.`;
	switch (about) {
		case "direct":
			return e.senderNotJoined ? notJoined : `Reply: ${direct}.`;
		case "topic":
			// Members cannot send to `all`: a broadcast is answered directly.
			if (e.topic === "all") return e.senderNotJoined ? notJoined : `Reply: ${direct}.`;
			return e.senderNotJoined ? `Reply: ${topic}. ${notJoined}` : `Reply: ${topic}, or ${direct}.`;
		case "thread":
			return `Earlier posts: bus_read_thread {space: "${e.space}", threadId: ${e.threadId}}. Answer: bus_add_post {space: "${e.space}", threadId: ${e.threadId}}.`;
		case "blackboard":
		case "other":
			return undefined;
	}
}

function openingTag(e: Entry): string {
	const at = isoMillis(e.createdAt);
	return at === undefined ? "<agent-bus-entry>" : `<agent-bus-entry at="${at}">`;
}

/** One fenced entry with the given (already escaped) body and hint. */
function fence(e: Entry, body: string | undefined, hint: string | undefined): string {
	const header = `${e.space} · ${where(e)} · from ${e.from}${e.critical ? " · CRITICAL" : ""}`;
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

/** The first NOTICE_CHARS characters of a line, never half of a surrogate pair. */
function cutLine(line: string): string {
	const end = /[\uD800-\uDBFF]/.test(line[NOTICE_CHARS - 1]) ? NOTICE_CHARS - 1 : NOTICE_CHARS;
	return line.slice(0, end);
}

/** The body of a post in a notice: whole when short, else its head and how many lines were left out. */
function noticeBody(body: string): string {
	const lines = body.split("\n");
	if (lines.length <= NOTICE_LINES && body.length <= NOTICE_CHARS) return escapeBody(body);
	const whole = headOf(lines);
	const shown = whole.length > 0 ? whole : [cutLine(lines[0])];
	const left = lines.length - whole.length;
	const rest = `${left} more ${left === 1 ? "line" : "lines"} left out.`;
	return `${escapeBody(shown.join("\n"))}\n${rest}`;
}

/** The notice of the posts to one thread: every post fenced with its author, the hint once at the end. */
export function postNotice(entries: readonly Entry[]): string {
	const last = entries.length - 1;
	return entries
		.map((e, i) => fence(e, e.body === undefined ? undefined : noticeBody(e.body), i === last ? replyHint(e) : undefined))
		.join("\n");
}
