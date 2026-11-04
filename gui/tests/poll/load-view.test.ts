import { describe, expect, test } from "bun:test";
import type { Reads } from "../../src/api/reads.ts";
import { loadView } from "../../src/poll/load-view.ts";
import { spaceDetail, spaceList } from "../fixtures.ts";

const reads: Reads = {
	spaces: async () => spaceList,
	space: async (name) => ({ ...spaceDetail, overview: { ...spaceDetail.overview, space: name } }),
};

describe("loading the data of a route", () => {
	test("the space list loads the list", async () => {
		expect(await loadView(reads, { name: "spaces" })).toEqual({ kind: "spaces", list: spaceList });
	});

	test("a space loads its detail", async () => {
		const view = await loadView(reads, { name: "space", space: "other" });
		expect(view?.kind === "space" && view.detail.overview.space).toBe("other");
	});

	test("a page without data loads nothing", () => {
		expect(loadView(reads, { name: "not-found" })).toBeUndefined();
	});
});
