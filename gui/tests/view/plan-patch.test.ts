import { describe, expect, test } from "bun:test";
import { h, type VNode } from "../../src/view/h.ts";
import { planPatch, samePlace } from "../../src/view/plan-patch.ts";

const li = (key: string, text = key): VNode => h("li", { key }, text);

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

describe("planPatch", () => {
	test("an empty parent gets every child created", () => {
		expect(planPatch([], ["a", h("p", null)])).toStrictEqual({ reuse: [undefined, undefined], remove: [] });
	});

	test("the same children are reused in place, with their new content", () => {
		expect(planPatch([li("a"), li("b")], [li("a", "A"), li("b", "B")])).toStrictEqual({ reuse: [0, 1], remove: [] });
	});

	test("an inserted child is created between the reused ones", () => {
		expect(planPatch([li("a"), li("c")], [li("a"), li("b"), li("c")])).toStrictEqual({ reuse: [0, undefined, 1], remove: [] });
	});

	test("a dropped child is removed and the others are kept", () => {
		expect(planPatch([li("a"), li("b"), li("c")], [li("a"), li("c")])).toStrictEqual({ reuse: [0, 2], remove: [1] });
	});

	test("keyed children that swap places are moved, not recreated", () => {
		expect(planPatch([li("a"), li("b")], [li("b"), li("a")])).toStrictEqual({ reuse: [1, 0], remove: [] });
	});

	test("a child whose tag changed is replaced", () => {
		expect(planPatch([h("p", null)], [h("div", null)])).toStrictEqual({ reuse: [undefined], remove: [0] });
	});

	test("a node the patcher never rendered is removed", () => {
		expect(planPatch([undefined, "x"], ["y"])).toStrictEqual({ reuse: [1], remove: [0] });
	});

	test("unkeyed children of one tag are reused in order", () => {
		expect(planPatch([h("li", null, "1"), h("li", null, "2")], [h("li", null, "3")])).toStrictEqual({ reuse: [0], remove: [1] });
	});
});
