import { describe, expect, test } from "bun:test";
import { h } from "../../src/view/h.ts";
import { samePlace } from "../../src/view/mount.ts";

describe("samePlace", () => {
	test("two texts share a place", () => {
		expect(samePlace("a", "b")).toBe(true);
	});

	test("a text and an element do not", () => {
		expect(samePlace("a", h("p", null))).toBe(false);
		expect(samePlace(h("p", null), "a")).toBe(false);
	});

	test("elements share a place when tag and key match", () => {
		expect(samePlace(h("li", { key: "a" }), h("li", { key: "a" }, "changed"))).toBe(true);
		expect(samePlace(h("li", { key: "a" }), h("li", { key: "b" }))).toBe(false);
		expect(samePlace(h("li", null), h("p", null))).toBe(false);
	});

	test("nothing rendered before shares no place", () => {
		expect(samePlace(undefined, "a")).toBe(false);
	});
});
