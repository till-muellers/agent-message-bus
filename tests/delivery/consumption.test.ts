import { describe, expect, test } from "bun:test";
import { consumedEntryIds } from "../../extension/lib/delivery/consumption.ts";

describe("consumedEntryIds", () => {
	test("the ids of the agent-bus messages of a request", () => {
		const messages = [
			{ role: "user", content: "hello" },
			{ role: "custom", customType: "agent-bus", details: { entryIds: [7, 8] } },
			{ role: "custom", customType: "agent-bus", details: { entryIds: [9] } },
		];
		expect(consumedEntryIds(messages)).toEqual([7, 8, 9]);
	});

	test("other messages contribute nothing", () => {
		const messages = [
			{ role: "user", content: "hello" },
			{ role: "custom", customType: "other-extension", details: { entryIds: [7] } },
			{ role: "custom", customType: "agent-bus" },
			{ role: "custom", customType: "agent-bus", details: {} },
		];
		expect(consumedEntryIds(messages)).toEqual([]);
	});

	test("ids that are not numbers are ignored", () => {
		const messages = [{ role: "custom", customType: "agent-bus", details: { entryIds: [1, "2", null, 3] } }];
		expect(consumedEntryIds(messages)).toEqual([1, 3]);
	});

	test("a request without bus messages consumes nothing", () => {
		expect(consumedEntryIds([])).toEqual([]);
	});
});
