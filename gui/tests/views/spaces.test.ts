import { describe, expect, test } from "bun:test";
import { spacesView } from "../../src/views/spaces.ts";
import { spaceList } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

function rows(list = spaceList): string[][] {
	const [table] = findAll(spacesView(list), byTag("table"));
	return table ? findAll(table, byTag("tr")).map((tr) => tr.children.map(textOf)) : [];
}

describe("the space list", () => {
	test("lists every space with leader count, admin names, member count and open invites", () => {
		expect(rows()).toEqual([
			["Space", "Leaders", "Admins", "Members", "Open invites"],
			["demo", "2", "ops", "1", "1"],
			["other", "1", "", "0", "0"],
		]);
	});

	test("links each space to its page", () => {
		const links = findAll(spacesView(spaceList), byTag("a")).map((a) => a.attrs.href);
		expect(links).toEqual(["#/spaces/demo", "#/spaces/other"]);
	});

	test("without leader rights there is no invites column", () => {
		const list = { ...spaceList, spaces: spaceList.spaces.map(({ invites: _, ...rest }) => rest) };
		expect(rows(list)[0]).toEqual(["Space", "Leaders", "Admins", "Members"]);
	});

	test("rows are keyed by space name, in the server's order", () => {
		const [table] = findAll(spacesView(spaceList), byTag("table"));
		const keys = table ? findAll(table, byTag("tr")).map((tr) => tr.key) : [];
		expect(keys).toEqual(["head", "demo", "other"]);
	});

	test("no space says so", () => {
		const view = spacesView({ spaces: [], inboxWaiting: 0 });
		expect(findAll(view, byTag("table"))).toEqual([]);
		expect(textOf(view)).toContain("No spaces");
	});
});
