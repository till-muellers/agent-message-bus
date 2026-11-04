import { describe, expect, test } from "bun:test";
import { h } from "../../src/view/h.ts";

describe("h", () => {
	test("builds a node from tag, attributes and children", () => {
		expect(h("a", { href: "#/", class: "nav" }, "Spaces")).toEqual({
			tag: "a",
			key: undefined,
			attrs: { href: "#/", class: "nav" },
			on: {},
			children: ["Spaces"],
		});
	});

	test("keeps the key and the handlers apart from the attributes", () => {
		const click = (): void => {};
		const node = h("button", { key: "out", on: { click } });
		expect(node.key).toBe("out");
		expect(node.on).toEqual({ click });
		expect(node.attrs).toEqual({});
	});

	test("true is an empty attribute, false and undefined are none, numbers are text", () => {
		expect(h("input", { required: true, disabled: false, title: undefined, size: 3 }).attrs).toEqual({
			required: "",
			size: "3",
		});
	});

	test("flattens nested children and drops empty ones", () => {
		expect(h("p", null, ["a", ["b"]], null, false, undefined, 7).children).toEqual(["a", "b", "7"]);
	});
});
