import { describe, expect, test } from "bun:test";
import type { VNode } from "../../src/view/h.ts";
import type { Intents } from "../../src/views/intents.ts";
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

const calls: string[] = [];
const went = async (): Promise<boolean> => true;
const intents: Pick<Intents, "invite" | "send" | "ask" | "cancelInvite"> = {
	invite: went,
	send: went,
	ask: (c) => void calls.push(`ask ${JSON.stringify(c)}`),
	cancelInvite: (space, invite) => void calls.push(`cancel ${space} ${invite.id}`),
};

function click(view: VNode, key: string): void {
	findAll(view, (n) => n.key === key)[0]?.on.click?.(new Event("click"));
}

const member = { ...spaceDetail, mayManage: false, invites: undefined };

/** The keys of the page's keyed buttons (the form submit buttons have none). */
function buttonKeys(view: VNode): string[] {
	return findAll(view, (n) => n.tag === "button").flatMap((b) => (b.key === undefined ? [] : [b.key]));
}

describe("the space page", () => {
	const view = spaceView(spaceDetail, intents, true);

	test("is titled with the space name", () => {
		expect(findAll(view, byTag("h1")).map(textOf)).toEqual(["demo"]);
	});

	test("lists the participants in the server's order with role and connection, offline ones since when", () => {
		const shown = rows(view, "participants").map((r) => r.slice(0, 3).map((cell) => cell.replace(/\s+/g, " ")));
		expect(shown).toEqual([
			["Name", "Role", "Connected"],
			["ops", "admin", "no, since 2026-11-04 19:00"],
			["lead", "leader", "yes"],
			["worker", "member", "no, since 2026-11-04 19:45"],
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
		expect(rows(view, "invites").map((r) => r.slice(0, 3))).toEqual([
			["Name", "Invited by", "Expires"],
			["new", "lead", "2026-11-04 20:10"],
		]);
		const [time] = findAll(section(view, "invites") ?? view, byTag("time"));
		expect(time?.attrs.datetime).toBe("2026-11-04T20:10:00Z");
	});

	test("a session without leader rights sees no invites section", () => {
		expect(section(spaceView(member, intents, false), "invites")).toBeUndefined();
	});

	test("a leader session without invites still gets the manage UI from mayManage", () => {
		const leader = spaceView({ ...spaceDetail, invites: undefined }, intents, false);
		expect(buttonKeys(leader)).toEqual(["revoke-worker", "delete-space"]);
		expect(findAll(leader, (n) => n.key === "invite-form")).toHaveLength(1);
		expect(textOf(section(leader, "invites") ?? leader)).toContain("No open invites");
	});

	test("an unknown role is shown verbatim and gets no revoke button", () => {
		const participants = [{ name: "eve", role: "observer", connected: false }];
		const page = spaceView({ ...spaceDetail, overview: { ...spaceDetail.overview, participants } }, intents, true);
		expect(rows(page, "participants").map((r) => r.slice(0, 2))).toEqual([["Name", "Role"], ["eve", "observer"]]);
		expect(buttonKeys(page)).not.toContain("revoke-eve");
	});

	test("empty lists say so", () => {
		const empty = spaceView({ ...spaceDetail, overview: { ...spaceDetail.overview, blackboards: [], recentThreads: [] }, invites: [] }, intents, true);
		expect(textOf(section(empty, "blackboards") ?? empty)).toContain("No blackboards");
		expect(textOf(section(empty, "threads") ?? empty)).toContain("No threads");
		expect(textOf(section(empty, "invites") ?? empty)).toContain("No open invites");
	});

	test("with leader rights every leader and member has a revoke button that asks first", () => {
		calls.length = 0;
		expect(findAll(view, (n) => textOf(n) === "Revoke" && n.tag === "button").map((b) => b.key)).toEqual(["revoke-lead", "revoke-worker"]);
		click(view, "revoke-worker");
		expect(calls).toEqual([`ask ${JSON.stringify({ kind: "revoke", space: "demo", name: "worker", role: "member" })}`]);
	});

	test("each open invite has a cancel button by id", () => {
		calls.length = 0;
		click(view, "cancel-3f9a0c1d2e4b5a67");
		expect(calls).toEqual(["cancel demo 3f9a0c1d2e4b5a67"]);
	});

	test("with leader rights the page offers the invite form and a delete that asks first", () => {
		calls.length = 0;
		expect(findAll(view, (n) => n.key === "invite-form")).toHaveLength(1);
		click(view, "delete-space");
		expect(calls).toEqual([`ask ${JSON.stringify({ kind: "delete-space", space: "demo" })}`]);
	});

	test("without leader rights there is no revoke, invite or delete, even with invites in the state", () => {
		const page = spaceView({ ...spaceDetail, mayManage: false }, intents, false);
		expect(buttonKeys(page)).toEqual([]);
		expect(findAll(page, (n) => n.key === "invite-form")).toEqual([]);
	});

	test("everyone gets the send form for the space's topics", () => {
		expect(findAll(view, (n) => n.key === "send-form")).toHaveLength(1);
	});

	test("a leader session revokes members only: leaders cannot be revoked", () => {
		const leader = spaceView(spaceDetail, intents, false);
		expect(findAll(leader, (n) => n.tag === "button" && textOf(n) === "Revoke").map((b) => b.key)).toEqual(["revoke-worker"]);
	});

	test("without leader rights the send form offers neither all nor critical", () => {
		const page = spaceView(member, intents, false);
		expect(findAll(page, byTag("option")).map(textOf)).toEqual(["notes"]);
		expect(findAll(page, (n) => n.attrs.name === "critical")).toEqual([]);
	});
});
