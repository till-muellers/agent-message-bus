import { describe, expect, test } from "bun:test";
import { decodeEntries } from "../../src/api/decode-inbox.ts";
import { entries } from "../fixtures.ts";

const wire = [
	{ id: 4, space: "demo", type: "direct_message", from: "worker", critical: false, class: "queue", body: "need a decision", createdAt: "2026-11-04T20:00:00Z" },
	{
		id: 9,
		space: "demo",
		type: "thread_post",
		from: "lead",
		critical: true,
		class: "interrupt",
		blackboard: "b",
		threadId: 1,
		threadTitle: "plan",
		body: "reply",
		createdAt: "2026-11-04T20:01:00Z",
	},
];

describe("decoding the inbox", () => {
	test("the entries in the server's order, optional fields only when sent", () => {
		const decoded = decodeEntries(wire);
		expect(decoded).toEqual(entries);
		expect("topic" in (decoded[0] ?? {})).toBe(false);
	});

	test("an entry type the client does not know is kept as text", () => {
		const [entry] = decodeEntries([{ ...wire[0], type: "future_kind" }]);
		expect(entry?.type).toBe("future_kind");
	});

	test("an entry without a sender is refused", () => {
		expect(() => decodeEntries([{ ...wire[0], from: undefined }])).toThrow(expect.objectContaining({ code: "invalid_response" }));
	});
});
