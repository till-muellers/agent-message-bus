import { describe, expect, test } from "bun:test";
import { decodeEntries } from "../../src/api/decode-inbox.ts";
import { at, without, wire } from "./wire.ts";

const post = at(wire("inbox.json"), "0");
const direct = at(wire("inbox.json"), "1");

describe("decoding the inbox", () => {
	test("optional fields only when the server sent them", () => {
		const [, decoded] = decodeEntries([post, direct]);
		expect(decoded?.body).toBe("stop");
		expect("blackboard" in (decoded ?? {})).toBe(false);
	});

	test("an entry type the client does not know is kept as text", () => {
		const [entry] = decodeEntries([{ ...without(direct, "type"), type: "future_kind" }]);
		expect(entry?.type).toBe("future_kind");
	});

	test("a delivery class the client does not know is kept as text", () => {
		const [entry] = decodeEntries([{ ...without(direct, "class"), class: "later" }]);
		expect(entry?.class).toBe("later");
	});

	const broken: [string, unknown][] = [
		["a sender", without(direct, "from")],
		["a delivery class", without(direct, "class")],
	];
	for (const [what, json] of broken) {
		test(`an entry without ${what} is refused`, () => {
			expect(() => decodeEntries([json])).toThrow(expect.objectContaining({ code: "invalid_response" }));
		});
	}
});
