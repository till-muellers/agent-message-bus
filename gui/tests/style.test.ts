/**
 * The stylesheet rules the views rely on, for what no unit test of the VNode tree sees (there is no layout engine
 * here): the browser smoke in `docs/design/arch/gui.md` §7 checks them for real.
 */
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const STYLE = readFileSync(join(import.meta.dir, "..", "src", "style.css"), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");

/** The declarations of the first rule whose selector list contains selector. */
function declarations(selector: string): string {
	for (const [, selectors = "", body = ""] of STYLE.matchAll(/([^{}]+)\{([^}]*)\}/g)) {
		if (selectors.split(",").some((s) => s.trim() === selector)) return body.replace(/\s+/g, " ");
	}
	return "";
}

describe("the stylesheet", () => {
	test("a confirmation, a secret and a notice stick to the top of the viewport", () => {
		const rule = declarations(".outcome");
		expect(rule).toContain("position: sticky");
		expect(rule).toContain("top: 0");
		expect(rule).toMatch(/background: \S+/);
	});

	test("unbroken text wraps instead of widening the page", () => {
		expect(declarations("body")).toContain("overflow-wrap: break-word");
		expect(declarations("header p")).toContain("min-width: 0");
		expect(declarations('[role="alertdialog"] p')).toContain("min-width: 0");
	});

	test("a table wider than the page scrolls inside its box", () => {
		expect(declarations(".scroll-x")).toContain("overflow-x: auto");
	});
});
