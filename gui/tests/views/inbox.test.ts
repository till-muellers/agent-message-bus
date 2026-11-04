import { describe, expect, test } from "bun:test";
import { inboxView } from "../../src/views/inbox.ts";
import { entries } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("the inbox page", () => {
	const ops = { kind: "admin", name: "ops" } as const;
	const view = inboxView(ops, entries);

	test("names the agent it belongs to", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["Inbox of admin ops"]);
	});

	test("lists each entry in the server's order with space, sender, kind and body", () => {
		const rows = findAll(view, byTag("tr")).map((tr) => tr.children.map(textOf));
		expect(rows.map((r) => r.slice(1))).toEqual([
			["Space", "From", "Kind", "Message"],
			["demo", "worker", "direct message", "need a decision"],
			["demo", "lead", "critical thread post on plan", "reply"],
		]);
	});

	test("links a thread entry to its thread", () => {
		expect(findAll(view, byTag("a")).map((a) => a.attrs.href)).toEqual(["#/spaces/demo/threads/1"]);
	});

	test("offers no acknowledge button", () => {
		expect(findAll(view, byTag("button"))).toEqual([]);
	});

	test("an empty inbox shows no entry", () => {
		expect(findAll(inboxView(ops, []), byTag("tr"))).toEqual([]);
		expect(textOf(inboxView(ops, []))).toContain("No entries.");
	});
});
