import { describe, expect, test } from "bun:test";
import { hrefOf, parseRoute, sameRoute } from "../../src/router/routes.ts";
import type { Route } from "../../src/state/app-state.ts";

describe("hash routes", () => {
	const cases: [string, Route][] = [
		["#/", { name: "spaces" }],
		["#/spaces/demo", { name: "space", space: "demo" }],
		["#/spaces/demo/boards/design", { name: "board", space: "demo", board: "design" }],
		["#/spaces/demo/threads/42", { name: "thread", space: "demo", threadId: 42 }],
		["#/inbox", { name: "inbox" }],
		["#/admins", { name: "admins" }],
	];

	for (const [hash, route] of cases) {
		test(`${hash} is the ${route.name} route and links back to itself`, () => {
			expect(parseRoute(hash)).toEqual(route);
			expect(hrefOf(route)).toBe(hash);
		});
	}

	test("an empty hash is the space list", () => {
		expect(parseRoute("")).toEqual({ name: "spaces" });
		expect(parseRoute("#")).toEqual({ name: "spaces" });
	});

	test("percent-encoded segments are decoded before they are checked", () => {
		expect(parseRoute("#/spaces/%64emo")).toEqual({ name: "space", space: "demo" });
	});

	const notFound = [
		"#/nope",
		"#/spaces",
		"#/spaces/Demo",
		"#/spaces/-demo",
		"#/spaces/a b",
		`#/spaces/${"a".repeat(64)}`,
		"#/spaces/demo/",
		"#/spaces/demo/boards",
		"#/spaces/demo/boards/Design",
		"#/spaces/demo/threads/abc",
		"#/spaces/demo/threads/0",
		"#/spaces/demo/threads/42/extra",
		"#/spaces/%E0%A4%A",
		"#/inbox/x",
		"spaces/demo",
	];
	for (const hash of notFound) {
		test(`${hash} is not found`, () => {
			expect(parseRoute(hash)).toEqual({ name: "not-found" });
		});
	}

	test("routes are the same when they point at the same place", () => {
		expect(sameRoute({ name: "space", space: "demo" }, { name: "space", space: "demo" })).toBe(true);
		expect(sameRoute({ name: "space", space: "demo" }, { name: "space", space: "other" })).toBe(false);
		expect(sameRoute({ name: "not-found" }, { name: "spaces" })).toBe(false);
	});
});
