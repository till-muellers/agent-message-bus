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

	const broken: [string, unknown][] = [
		["spaces that are no list", { ...without(listWire, "spaces"), spaces: {} }],
		["a leader that is not text", listWith([{ ...without(row, "leaders"), leaders: [1] }])],
		["a negative member count", listWith([{ ...without(row, "members"), members: -1 }])],
		["a fractional invite count", listWith([{ ...without(row, "invites"), invites: 1.5 }])],
		["joined that is not a flag", listWith([{ ...without(row, "joined"), joined: "no" }])],
		["no inbox", without(listWire, "inbox")],
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
	const broken: [string, unknown][] = [
		["no overview", without(detailWire, "overview")],
		["an unknown role", withOverview("participants", [{ name: "x", role: "boss", connected: false }])],
		["a topic without creator", withOverview("topics", [{ name: "all", subscribed: false }])],
		["a thread id that is not a number", withOverview("recentThreads", [{ ...without(at(overview, "recentThreads", "0"), "id"), id: "7" }])],
		["an invite without id", { ...without(detailWire, "invites"), invites: [without(at(detailWire, "invites", "0"), "id")] }],
	];
	for (const [what, json] of broken) {
		test(`a detail with ${what} is an invalid response`, () => {
			expect(() => decodeSpaceDetail(json)).toThrow(invalid());
		});
	}
});
