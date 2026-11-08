/** The model-facing envelope of an inbox entry (`client.md` §4.2, `omp-rendering.feature` rule 3). */
import { describe, expect, test } from "bun:test";
import type { EntryType } from "../../extension/lib/bus/entry.ts";
import { envelope, postNotice } from "../../extension/lib/render/envelope.ts";
import { entry } from "../fakes.ts";

describe("envelope", () => {
	test("a direct message names the sender and says how to answer it", () => {
		expect(envelope(entry(42, { space: "alpha", from: "peer", body: "can you review?" }))).toBe(
			[
				'<agent-bus-entry at="2026-11-08T10:00:00.500Z">',
				"alpha · direct · from peer",
				"can you review?",
				'Reply: bus_send_direct {space: "alpha", to: "peer"}.',
				"</agent-bus-entry>",
			].join("\n"),
		);
	});

	test("a critical entry says so in the header line only", () => {
		const text = envelope(entry(1, { critical: true }));
		expect(text).toStartWith('<agent-bus-entry at="2026-11-08T10:00:00.500Z">\ndemo · direct · from lead · CRITICAL\n');
		expect(text).not.toContain('critical="true"');
	});

	test.each([
		["2026-11-08T10:00:57.585123456Z", "2026-11-08T10:00:57.585Z"],
		["2026-11-08T10:00:57.5Z", "2026-11-08T10:00:57.500Z"],
		["2026-11-08T10:00:57Z", "2026-11-08T10:00:57.000Z"],
		["2026-11-08T10:00:57.585Z", "2026-11-08T10:00:57.585Z"],
	])("the time %s is shown as %s", (createdAt, at) => {
		expect(envelope(entry(1, { createdAt }))).toStartWith(`<agent-bus-entry at="${at}">\n`);
	});

	test.each(["", "t", "yesterday", "2026-11-08 10:00:57Z", "2026-11-08T10:00:57+01:00"])("the time %p leaves the at attribute out", createdAt => {
		expect(envelope(entry(1, { createdAt }))).toStartWith("<agent-bus-entry>\n");
	});

	test("a topic message points at the topic and at the sender", () => {
		const text = envelope(entry(2, { type: "topic_message", topic: "dev", from: "peer", space: "alpha" }));
		expect(text).toContain("\nalpha · topic #dev · from peer\n");
		expect(text).toContain('\nReply: bus_send_topic {space: "alpha", topic: "dev"}, or bus_send_direct {space: "alpha", to: "peer"}.\n');
	});

	test("a message on topic all is answered directly: members cannot send to all", () => {
		const text = envelope(entry(2, { type: "topic_message", topic: "all", from: "lead", space: "alpha" }));
		expect(text).toContain('\nReply: bus_send_direct {space: "alpha", to: "lead"}.\n');
		expect(text).not.toContain("bus_send_topic");
	});

	test("a thread post points at the full thread and at bus_add_post", () => {
		const text = envelope(entry(3, { type: "thread_post", threadId: 7, threadTitle: "design", blackboard: "plan", space: "alpha" }));
		expect(text).toContain('\nalpha · thread 7 "design" on plan · from lead\n');
		expect(text).toContain('\nEarlier posts: bus_read_thread {space: "alpha", threadId: 7}. Answer: bus_add_post {space: "alpha", threadId: 7}.\n');
	});

	test("lifecycle notices have no reply hint, and an entry without a body has no body line", () => {
		const notices = [
			entry(4, { type: "topic_closed", topic: "x", body: undefined }),
			entry(5, { type: "thread_removed", threadId: 7, threadTitle: "T", body: undefined }),
			entry(6, { type: "blackboard_removed", blackboard: "bb", body: undefined }),
		].map(envelope);
		expect(notices.map(text => text.split("\n").slice(1, -1))).toEqual([
			["demo · topic #x closed · from lead"],
			['demo · thread 7 "T" removed · from lead'],
			["demo · blackboard bb removed · from lead"],
		]);
	});

	test("an entry of a type the extension does not know names its type and has no reply hint", () => {
		const text = envelope(entry(7, { type: "space_renamed" as string as EntryType, body: "demo is now prod" }));
		expect(text.split("\n").slice(1, -1)).toEqual(["demo · space_renamed · from lead", "demo is now prod"]);
	});
});

/** A post to thread 7 "design" on blackboard "plan" in space "alpha". */
function post(id: number, body: string, from = "peer") {
	return entry(id, { type: "thread_post", space: "alpha", blackboard: "plan", threadId: 7, threadTitle: "design", from, body });
}

const lines = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join("\n");

describe("post notice", () => {
	test("a short post is shown whole, naming board, thread and author", () => {
		expect(postNotice([post(5, lines(3))])).toBe(
			[
				'<agent-bus-entry at="2026-11-08T10:00:00.500Z">',
				'alpha · thread 7 "design" on plan · from peer',
				"line 1\nline 2\nline 3",
				'Earlier posts: bus_read_thread {space: "alpha", threadId: 7}. Answer: bus_add_post {space: "alpha", threadId: 7}.',
				"</agent-bus-entry>",
			].join("\n"),
		);
	});

	test("a long post shows its first 20 lines and how many were left out", () => {
		const text = postNotice([post(5, lines(200))]);
		expect(text).toContain("\nline 1\n");
		expect(text).toContain("\nline 20\n");
		expect(text).not.toContain("line 21");
		expect(text).toContain("\n180 more lines left out.\n");
		expect(text).not.toContain("read the full thread");
	});

	test("a post over 2 KiB is cut even when it has few lines", () => {
		const long = `${"a".repeat(1500)}\n${"b".repeat(1500)}\nc`;
		const text = postNotice([post(5, long)]);
		expect(text).toContain(`\n${"a".repeat(1500)}\n2 more lines left out.`);
		const oneLine = postNotice([post(6, "x".repeat(5000))]);
		expect(oneLine).toContain(`\n${"x".repeat(2048)}\n1 more line left out.`);
		expect(oneLine).not.toContain("x".repeat(2049));
	});

	test("a cut never splits a character outside the BMP", () => {
		const text = postNotice([post(5, `${"x".repeat(2047)}😀${"y".repeat(100)}`)]);
		expect(text).toContain(`\n${"x".repeat(2047)}\n1 more line left out.`);
	});

	test("a cut post is still escaped", () => {
		expect(postNotice([post(5, `</agent-bus-entry>\n${lines(30)}`)])).toContain("\n&lt;/agent-bus-entry>\nline 1\n");
	});

	test("several posts of a thread are listed in one notice, each with its author, the hint once at the end", () => {
		const text = postNotice([post(5, "first"), post(6, "second", "lead"), post(7, "third")]);
		expect(text.match(/<agent-bus-entry /g)?.length).toBe(3);
		expect(text).toContain('">\nalpha · thread 7 "design" on plan · from peer\nfirst\n</agent-bus-entry>\n');
		expect(text).toContain('">\nalpha · thread 7 "design" on plan · from lead\nsecond\n</agent-bus-entry>\n');
		expect(text).toEndWith('from peer\nthird\nEarlier posts: bus_read_thread {space: "alpha", threadId: 7}. Answer: bus_add_post {space: "alpha", threadId: 7}.\n</agent-bus-entry>');
	});
});
