import { describe, expect, test } from "bun:test";
import type { Reads } from "../../src/api/reads.ts";
import { loadView } from "../../src/poll/load-view.ts";
import { entries, spaceDetail, spaceList, thread, threadInfos } from "../fixtures.ts";

const reads: Reads = {
	spaces: async () => spaceList,
	space: async (name) => ({ ...spaceDetail, overview: { ...spaceDetail.overview, space: name } }),
	threads: async () => threadInfos,
	thread: async () => thread,
	inbox: async () => entries,
};

describe("loading the data of a route", () => {
	test("the space list loads the list", async () => {
		expect(await loadView(reads, { name: "spaces" })).toEqual({ kind: "spaces", list: spaceList });
	});

	test("a space loads its detail", async () => {
		const view = await loadView(reads, { name: "space", space: "other" });
		expect(view?.kind === "space" && view.detail.overview.space).toBe("other");
	});

	test("a blackboard loads its threads with its space and name", async () => {
		const view = await loadView(reads, { name: "board", space: "demo", board: "b" });
		expect(view).toEqual({ kind: "board", space: "demo", board: "b", threads: threadInfos });
	});

	test("a thread loads the thread with its space", async () => {
		expect(await loadView(reads, { name: "thread", space: "demo", threadId: 1 })).toEqual({ kind: "thread", space: "demo", thread });
	});

	test("the inbox loads the entries", async () => {
		expect(await loadView(reads, { name: "inbox" })).toEqual({ kind: "inbox", entries });
	});

	test("a page without data loads nothing", () => {
		expect(loadView(reads, { name: "not-found" })).toBeUndefined();
	});
});
