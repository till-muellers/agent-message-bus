import { describe, expect, test } from "bun:test";
import { decodeSpaceDetail, decodeSpaceList } from "../../src/api/decode-state.ts";
import { at, without, wire } from "./wire.ts";

const listWire = wire("state-list.json");
const row = at(listWire, "spaces", "0");

function listWith(spaces: unknown[]): unknown {
	return { ...without(listWire, "spaces"), spaces };
}

function invalid(): unknown {
	return expect.objectContaining({ code: "invalid_response" });
}

describe("decoding the space list", () => {
	test("a space without an invites count has none (no leader rights)", () => {
		const [demo] = decodeSpaceList(listWith([without(row, "invites")])).spaces;
		expect(demo?.invites).toBeUndefined();
	});

	test("the list carries whether the session may manage each space and the server's inbox limit", () => {
		const managed = { ...without(row, "mayManage"), mayManage: true };
		const notManaged = { ...without(row, "invites"), name: "x", mayManage: false };
		const list = decodeSpaceList({ ...without(listWire, "maxInboxLimit"), spaces: [managed, notManaged], maxInboxLimit: 250 });
		expect(list.spaces.map((s) => s.mayManage)).toEqual([true, false]);
		expect(list.maxInboxLimit).toBe(250);
	});

	const broken: [string, unknown][] = [
		["spaces that are no list", { ...without(listWire, "spaces"), spaces: {} }],
		["a leader that is not text", listWith([{ ...without(row, "leaders"), leaders: [1] }])],
		["a negative member count", listWith([{ ...without(row, "members"), members: -1 }])],
		["a fractional invite count", listWith([{ ...without(row, "invites"), invites: 1.5 }])],
		["joined that is not a flag", listWith([{ ...without(row, "joined"), joined: "no" }])],
		["a row without mayManage", listWith([without(row, "mayManage")])],
		["no inbox", without(listWire, "inbox")],
		["no inbox limit", without(listWire, "maxInboxLimit")],
	];
	for (const [what, json] of broken) {
		test(`a list with ${what} is an invalid response`, () => {
			expect(() => decodeSpaceList(json)).toThrow(invalid());
		});
	}
});

describe("decoding the space detail", () => {
	const detailWire = wire("state-space-leader.json");
	const overview = without(at(detailWire, "overview"), "you");
	const withOverview = (key: string, value: unknown): unknown => ({
		...without(detailWire, "overview"),
		overview: { ...overview, [key]: value },
	});

	test("a role the GUI does not know decodes as text", () => {
		const detail = decodeSpaceDetail(withOverview("participants", [{ name: "x", role: "boss", connected: false }]));
		expect(detail.overview.participants).toEqual([{ name: "x", role: "boss", connected: false }]);
	});

	test("a participant that is not connected keeps its offlineSince", () => {
		const offline = { name: "x", role: "member", connected: false, offlineSince: "2026-11-04T19:00:00Z" };
		expect(decodeSpaceDetail(withOverview("participants", [offline])).overview.participants).toEqual([offline]);
	});

	const broken: [string, unknown][] = [
		["no overview", without(detailWire, "overview")],
		["an offlineSince that is not text", withOverview("participants", [{ name: "x", role: "member", connected: false, offlineSince: 1 }])],
		["a role that is not text", withOverview("participants", [{ name: "x", role: 1, connected: false }])],
		["a topic without creator", withOverview("topics", [{ name: "all", subscribed: false }])],
		["a thread id that is not a number", withOverview("recentThreads", [{ ...without(at(overview, "recentThreads", "0"), "id"), id: "7" }])],
		["an invite without id", { ...without(detailWire, "invites"), invites: [without(at(detailWire, "invites", "0"), "id")] }],
		["no mayManage", without(detailWire, "mayManage")],
	];
	for (const [what, json] of broken) {
		test(`a detail with ${what} is an invalid response`, () => {
			expect(() => decodeSpaceDetail(json)).toThrow(invalid());
		});
	}
});
