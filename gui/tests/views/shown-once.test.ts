import { describe, expect, test } from "bun:test";
import { shownOnceView } from "../../src/views/shown-once.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("a secret shown once", () => {
	const view = shownOnceView({ label: "Token of admin ops", secret: "amb_admin_x" });

	test("names what the secret is and shows it as code", () => {
		expect(textOf(view)).toContain("Token of admin ops");
		expect(findAll(view, byTag("code")).map(textOf)).toEqual(["amb_admin_x"]);
	});

	test("says it cannot be shown again", () => {
		expect(textOf(view)).toContain("it cannot be shown again");
	});

	test("takes the focus when it appears and stays in view while the page scrolls", () => {
		expect(view.attrs).toMatchObject({ autofocus: "", tabindex: "-1", class: "outcome" });
	});

	test("another secret is another node, so it takes the focus again", () => {
		const next = shownOnceView({ label: "Token of admin ops", secret: "amb_admin_y" });
		expect(next.key).not.toBe(view.key);
	});

	test("can be hidden without leaving the page, and is shown by default", () => {
		const secret = { label: "Token of admin ops", secret: "amb_admin_x" };
		expect(shownOnceView(secret, true).attrs.hidden).toBe("");
		expect(shownOnceView(secret, true).key).toBe(view.key);
		expect(view.attrs.hidden).toBeUndefined();
	});
});
