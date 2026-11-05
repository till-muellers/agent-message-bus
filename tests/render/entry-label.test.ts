/** What an inbox entry is about and which entries are posts (`client.md` §3.5, §4.2-4.3). */
import { describe, expect, test } from "bun:test";
import type { EntryType } from "../../extension/lib/bus/entry.ts";
import { type EntryLabel, isPost, labelOf } from "../../extension/lib/render/entry-label.ts";
import { entry } from "../fakes.ts";

const LABELS: [EntryType, EntryLabel][] = [
	["direct_message", { about: "direct", ended: false }],
	["topic_message", { about: "topic", ended: false }],
	["topic_closed", { about: "topic", ended: true }],
	["thread_post", { about: "thread", ended: false }],
	["thread_removed", { about: "thread", ended: true }],
	["blackboard_removed", { about: "blackboard", ended: true }],
];

describe("the entry label", () => {
	test("every entry type is about a direct message, a topic, a thread or a board, and a lifecycle notice ends it", () => {
		for (const [type, label] of LABELS) expect({ type, label: labelOf(entry(1, { type })) }).toEqual({ type, label });
	});

	test("a post is a thread entry of a class that is not deferred, whatever its type", () => {
		const thread = { blackboard: "plan", threadId: 7 };
		expect(isPost(entry(1, { type: "thread_post", ...thread }))).toBe(true);
		expect(isPost(entry(2, { type: "thread_post", ...thread, class: "interrupt", critical: true }))).toBe(true);
		expect(isPost(entry(3, { type: "thread_post", ...thread, class: undefined }))).toBe(true);
		expect(isPost(entry(4, { type: "thread_post", ...thread, class: "deferred" }))).toBe(false);
		expect(isPost(entry(5, { type: "thread_removed", ...thread, class: "deferred" }))).toBe(false);
		expect(isPost(entry(6, { type: "direct_message" }))).toBe(false);
	});
});
