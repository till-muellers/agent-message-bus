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
});
