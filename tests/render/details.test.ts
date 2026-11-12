/** The agent-bus session message: the envelope as content and the entry as details (`client.md` §4.1). */
import { describe, expect, test } from "bun:test";
import { busMessage } from "../../extension/lib/render/details.ts";
import { envelope } from "../../extension/lib/render/envelope.ts";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { entry } from "../fakes.ts";

describe("busMessage", () => {
	test("a delivered entry is an agent-bus message with the envelope and the entry fields as details", () => {
		const dm = entry(1, { from: "peer", body: "hi" });
		expect(busMessage(dm)).toEqual({
			customType: "agent-bus",
			content: envelope(dm),
			display: true,
			details: {
				entryIds: [1],
				entries: [{ id: 1, space: "demo", type: "direct_message", from: "peer", critical: false, class: "queue", createdAt: "2026-11-08T10:00:00.5Z", body: "hi" }],
			},
		});
	});

	test("topic, blackboard, threadId, threadTitle and postId are in the stored details when the entry has them", () => {
		const stored = (e: Entry) => JSON.parse(JSON.stringify(busMessage(e).details.entries[0])) as Record<string, unknown>;
		expect(stored(entry(2, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "design", postId: 27 }))).toMatchObject({ blackboard: "plan", threadId: 7, threadTitle: "design", postId: 27 });
		expect(stored(entry(3, { type: "topic_message", topic: "dev" }))).toMatchObject({ topic: "dev" });
		expect(Object.keys(stored(entry(4)))).not.toContain("topic");
	});

	test("fields the extension does not know stay out of the details", () => {
		const wire = { ...entry(5), edge: true } as Entry;
		expect(Object.keys(busMessage(wire).details.entries[0])).not.toContain("edge");
	});
});
