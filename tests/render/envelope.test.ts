import { describe, expect, test } from "bun:test";
import { formatEntry } from "../../extension/lib/render/envelope.ts";
import { entry } from "../fakes.ts";

describe("formatEntry", () => {
	test("renders each type", () => {
		expect(formatEntry(entry(1, { critical: true }))).toBe("[agent-bus] demo · direct · from lead · CRITICAL\nbody 1");
		expect(formatEntry(entry(2, { type: "topic_message", topic: "all" }))).toStartWith("[agent-bus] demo · topic #all · from lead\n");
		expect(formatEntry(entry(3, { type: "topic_closed", topic: "x" }))).toContain("· topic #x closed ·");
		expect(formatEntry(entry(4, { type: "thread_post", threadId: 7, threadTitle: "T", blackboard: "bb" }))).toContain('· thread 7 "T" on bb ·');
		expect(formatEntry(entry(5, { type: "thread_removed", threadId: 7, threadTitle: "T" }))).toContain('· thread 7 "T" removed ·');
		expect(formatEntry(entry(6, { type: "blackboard_removed", blackboard: "bb" }))).toContain("· blackboard bb removed ·");
	});
});
