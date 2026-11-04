import { describe, expect, test } from "bun:test";
import { inboxView } from "../../src/views/inbox.ts";
import { entries } from "../fixtures.ts";
import type { VChild } from "../../src/view/h.ts";
import type { Entry } from "../../src/api/types.ts";
import { byTag, findAll, textOf } from "../tree.ts";

/** The text a browser shows: children joined without added spaces. */
function shown(node: VChild): string {
	return typeof node === "string" ? node : node.children.map(shown).join("");
}

function rowsOf(list: Entry[]): string[][] {
	return findAll(inboxView({ kind: "admin", name: "ops" }, list), byTag("tr")).map((tr) => tr.children.map(shown));
}

const base: Entry = { id: 1, space: "demo", type: "topic_closed", from: "lead", critical: false, createdAt: "2026-11-04T20:00:00Z" };

describe("the inbox page", () => {
	const ops = { kind: "admin", name: "ops" } as const;
	const view = inboxView(ops, entries);

	test("names the agent it belongs to", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["Inbox of admin ops"]);
	});

	test("lists each entry in the server's order with space, sender, kind and body", () => {
		expect(rowsOf(entries).map((r) => r.slice(1))).toEqual([
			["Space", "From", "Kind", "Message"],
			["demo", "worker", "direct message", "need a decision"],
			["demo", "lead", "critical thread post on plan", "reply"],
		]);
	});

	test("names the topic, the removed thread or the removed blackboard an entry concerns", () => {
		const list: Entry[] = [
			{ ...base, id: 1, topic: "notes" },
			{ ...base, id: 2, type: "thread_removed", threadId: 3, threadTitle: "old" },
			{ ...base, id: 3, type: "blackboard_removed", blackboard: "b" },
		];
		expect(rowsOf(list).slice(1).map((r) => r[3])).toEqual(["topic closed on notes", "thread removed on old", "blackboard removed on b"]);
		expect(findAll(inboxView({ kind: "admin", name: "ops" }, list), byTag("a"))).toEqual([]);
	});

	test("says when the list is cut at the read limit", () => {
		const many = Array.from({ length: 500 }, (_, i) => ({ ...base, id: i + 1 }));
		expect(textOf(inboxView({ kind: "admin", name: "ops" }, many))).toContain("Showing the oldest 500 entries");
		expect(textOf(view)).not.toContain("Showing the oldest");
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
