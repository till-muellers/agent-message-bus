import { describe, expect, test } from "bun:test";
import type { VNode } from "../../src/view/h.ts";
import { adminsView } from "../../src/views/admins.ts";
import { admins } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

function view(calls: string[] = []): VNode {
	return adminsView(admins, { mintAdmin: (name) => void calls.push(`mint ${name}`), revokeAdmin: (name) => void calls.push(`revoke ${name}`) });
}

describe("the admin area", () => {
	test("lists every admin with bootstrap flag, tokens and joined spaces", () => {
		const rows = findAll(view(), byTag("tr")).map((tr) => tr.children.slice(0, 4).map(textOf));
		expect(rows).toEqual([
			["Name", "Bootstrap", "Tokens", "Spaces"],
			["admin", "yes", "0", ""],
			["ops", "no", "2", "demo, other"],
		]);
	});

	test("offers a revoke button for minted admins only", () => {
		const calls: string[] = [];
		const buttons = findAll(view(calls), (n) => n.tag === "button" && textOf(n) === "Revoke");
		expect(buttons).toHaveLength(1);
		buttons[0]?.on.click?.(new Event("click"));
		expect(calls).toEqual(["revoke ops"]);
	});

	test("offers a mint form with a name field", () => {
		const [form] = findAll(view(), byTag("form"));
		expect(form && findAll(form, byTag("input")).map((i) => i.attrs.name)).toEqual(["name"]);
		expect(form && textOf(form)).toContain("Mint admin");
		expect(typeof form?.on.submit).toBe("function");
	});
});
