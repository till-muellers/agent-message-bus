/** The model-facing envelope of an inbox entry (`client.md` §4.2, `omp-rendering.feature` rule 3). */
import { describe, expect, test } from "bun:test";
import { envelope } from "../../extension/lib/render/envelope.ts";
import { entry } from "../fakes.ts";

describe("envelope", () => {
	test("a direct message names the sender and says how to answer it", () => {
		expect(envelope(entry(42, { space: "alpha", from: "peer", body: "can you review?" }))).toBe(
			[
				'<agent-bus-entry space="alpha" type="direct_message" from="peer" id="42">',
				"[agent-bus] alpha · direct · from peer",
				"can you review?",
				'Reply: bus_send_direct {space: "alpha", to: "peer"}.',
				"</agent-bus-entry>",
			].join("\n"),
		);
	});

	test("a critical entry says so on the tag and in the header line", () => {
		const text = envelope(entry(1, { critical: true }));
		expect(text).toStartWith('<agent-bus-entry space="demo" type="direct_message" from="lead" id="1" critical="true">\n[agent-bus] demo · direct · from lead · CRITICAL\n');
	});

	test("a topic message points at the topic and at the sender", () => {
		const text = envelope(entry(2, { type: "topic_message", topic: "dev", from: "peer", space: "alpha" }));
		expect(text).toContain("\n[agent-bus] alpha · topic #dev · from peer\n");
		expect(text).toContain('\nReply: bus_send_topic {space: "alpha", topic: "dev"}, or bus_send_direct {space: "alpha", to: "peer"}.\n');
	});

	test("a message on topic all is answered directly: members cannot send to all", () => {
		const text = envelope(entry(2, { type: "topic_message", topic: "all", from: "lead", space: "alpha" }));
		expect(text).toContain('\nReply: bus_send_direct {space: "alpha", to: "lead"}.\n');
		expect(text).not.toContain("bus_send_topic");
	});

	test("a thread post points at the full thread and at bus_add_post", () => {
		const text = envelope(entry(3, { type: "thread_post", threadId: 7, threadTitle: "design", blackboard: "plan", space: "alpha" }));
		expect(text).toContain('\n[agent-bus] alpha · thread 7 "design" on plan · from lead\n');
		expect(text).toContain('\nFull thread: bus_read_thread {space: "alpha", threadId: 7}. Answer: bus_add_post {space: "alpha", threadId: 7}.\n');
	});

	test("lifecycle notices have no reply hint, and an entry without a body has no body line", () => {
		const notices = [
			entry(4, { type: "topic_closed", topic: "x", body: undefined }),
			entry(5, { type: "thread_removed", threadId: 7, threadTitle: "T", body: undefined }),
			entry(6, { type: "blackboard_removed", blackboard: "bb", body: undefined }),
		].map(envelope);
		expect(notices.map(text => text.split("\n").slice(1, -1))).toEqual([
			["[agent-bus] demo · topic #x closed · from lead"],
			['[agent-bus] demo · thread 7 "T" removed · from lead'],
			["[agent-bus] demo · blackboard bb removed · from lead"],
		]);
	});
});
