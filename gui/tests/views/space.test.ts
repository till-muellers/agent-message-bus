import { describe, expect, test } from "bun:test";
import type { VNode } from "../../src/view/h.ts";
import { spaceView } from "../../src/views/space.ts";
import { spaceDetail } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

function section(view: VNode, title: string): VNode | undefined {
	return findAll(view, byTag("section")).find((s) => s.key === title);
}

function rows(view: VNode, title: string): string[][] {
	const s = section(view, title);
	return s ? findAll(s, byTag("tr")).map((tr) => tr.children.map(textOf)) : [];
}

describe("the space page", () => {
	const view = spaceView(spaceDetail);

	test("is titled with the space name", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["demo"]);
	});

	test("lists the participants in the server's order with role and connection", () => {
		expect(rows(view, "participants")).toEqual([
			["Name", "Role", "Connected"],
			["ops", "admin", "no"],
			["lead", "leader", "yes"],
			["worker", "member", "no"],
		]);
	});

	test("lists the topics with their creator", () => {
		expect(rows(view, "topics")).toEqual([
			["Topic", "Created by"],
			["all", "lead"],
			["notes", "worker"],
		]);
	});

	test("lists the blackboards with owner and thread count, each linked to its threads", () => {
		expect(rows(view, "blackboards")).toEqual([
			["Blackboard", "Owner", "Threads"],
			["alpha", "worker", "0"],
			["zeta", "worker", "1"],
		]);
		const links = findAll(section(view, "blackboards") ?? view, byTag("a")).map((a) => a.attrs.href);
		expect(links).toEqual(["#/spaces/demo/boards/alpha", "#/spaces/demo/boards/zeta"]);
	});

	test("lists the recent threads linked to the thread", () => {
		expect(rows(view, "threads")).toEqual([
			["Thread", "Blackboard", "Posts", "Last post"],
			["plan", "zeta", "2", "2026-11-04 20:00"],
		]);
		const links = findAll(section(view, "threads") ?? view, byTag("a")).map((a) => a.attrs.href);
		expect(links).toEqual(["#/spaces/demo/threads/7"]);
	});

	test("lists the open invites with invited name, inviter and expiry, never a code", () => {
		expect(rows(view, "invites")).toEqual([
			["Name", "Invited by", "Expires"],
			["new", "lead", "2026-11-04 20:10"],
		]);
		const [time] = findAll(section(view, "invites") ?? view, byTag("time"));
		expect(time?.attrs.datetime).toBe("2026-11-04T20:10:00Z");
	});

	test("a session without leader rights sees no invites section", () => {
		expect(section(spaceView({ ...spaceDetail, invites: undefined }), "invites")).toBeUndefined();
	});

	test("empty lists say so", () => {
		const empty = spaceView({ ...spaceDetail, overview: { ...spaceDetail.overview, blackboards: [], recentThreads: [] }, invites: [] });
		expect(textOf(section(empty, "blackboards") ?? empty)).toContain("No blackboards");
		expect(textOf(section(empty, "threads") ?? empty)).toContain("No threads");
		expect(textOf(section(empty, "invites") ?? empty)).toContain("No open invites");
	});
});
