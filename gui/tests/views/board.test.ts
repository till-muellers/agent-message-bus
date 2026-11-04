import { describe, expect, test } from "bun:test";
import { boardView } from "../../src/views/board.ts";
import { threadInfos } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("the blackboard page", () => {
	const view = boardView("demo", "b", threadInfos);

	test("is titled with the blackboard and links back to its space", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["b"]);
		expect(findAll(view, byTag("a")).map((a) => a.attrs.href)).toContain("#/spaces/demo");
	});

	test("lists the threads in the server's order with author, posts and latest post time", () => {
		const rows = findAll(view, byTag("tr")).map((tr) => tr.children.map(textOf));
		expect(rows.map((r) => r.slice(0, 4))).toEqual([
			["Id", "Thread", "Created by", "Posts"],
			["2", "other", "worker2", "1"],
			["1", "plan", "worker", "1"],
		]);
		expect(findAll(view, byTag("time")).map((t) => t.attrs.datetime)).toEqual(["2026-11-04T20:05:00Z", "2026-11-04T20:00:00Z"]);
	});

	test("links each thread to its page", () => {
		expect(findAll(view, byTag("a")).map((a) => a.attrs.href)).toContain("#/spaces/demo/threads/1");
	});

	test("a blackboard without threads says so", () => {
		expect(textOf(boardView("demo", "b", []))).toContain("No threads.");
	});
});
