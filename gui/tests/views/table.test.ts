import { describe, expect, test } from "bun:test";
import { table } from "../../src/views/table.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("a table", () => {
	const view = table(["Name", "Role"], [{ key: "a", cells: ["lead", "leader"] }]);

	test("has a header row and a keyed row per entry", () => {
		expect(findAll(view, byTag("tr")).map((tr) => tr.children.map(textOf))).toEqual([["Name", "Role"], ["lead", "leader"]]);
		expect(findAll(view, byTag("tr")).map((tr) => tr.key)).toEqual(["head", "a"]);
	});

	test("scrolls sideways inside its own box when a long name is wider than the page", () => {
		expect(view.tag).toBe("div");
		expect(view.attrs.class).toBe("scroll-x");
		expect(view.children.map((child) => (typeof child === "string" ? child : child.tag))).toEqual(["table"]);
	});
});
