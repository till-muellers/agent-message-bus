import { describe, expect, test } from "bun:test";
import { sameData } from "../../src/state/same-data.ts";

describe("sameData", () => {
	test("equal JSON values are the same", () => {
		expect(sameData({ a: [1, "x", { b: true }], c: null }, { a: [1, "x", { b: true }], c: null })).toBe(true);
	});

	test("key order does not matter", () => {
		expect(sameData({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true);
	});

	const different: [string, unknown, unknown][] = [
		["a changed number", { a: 1 }, { a: 2 }],
		["an extra key", { a: 1 }, { a: 1, b: 2 }],
		["a missing key", { a: 1, b: undefined }, { a: 1, c: undefined }],
		["a longer list", [1], [1, 2]],
		["a list and an object", [], {}],
		["null and an object", null, {}],
	];
	for (const [what, a, b] of different) {
		test(`${what} is different`, () => {
			expect(sameData(a, b)).toBe(false);
		});
	}
});
