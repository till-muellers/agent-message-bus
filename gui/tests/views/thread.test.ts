import { describe, expect, test } from "bun:test";
import { threadView } from "../../src/views/thread.ts";
import { thread } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("the thread page", () => {
	const view = threadView("demo", thread, { post: () => {} });

	test("shows the title, its blackboard and its author", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["plan"]);
		expect(textOf(view).replace(/\s+/g, " ")).toContain("on blackboard b created by worker");
		expect(findAll(view, byTag("a")).map((a) => a.attrs.href)).toContain("#/spaces/demo/boards/b");
	});

	test("lists the posts in order with author, time and body", () => {
		const posts = findAll(view, byTag("article"));
		expect(posts.map((p) => p.key)).toEqual(["1", "2"]);
		expect(posts.map((p) => findAll(p, byTag("strong")).map(textOf))).toEqual([["worker"], ["lead"]]);
		expect(posts.map((p) => findAll(p, byTag("time")).length)).toEqual([1, 1]);
		expect(posts.map((p) => findAll(p, byTag("p")).map(textOf))).toEqual([["first"], ["reply"]]);
	});

	test("ends with the reply form", () => {
		expect(findAll(view, (n) => n.key === "post-form")).toHaveLength(1);
	});
});
