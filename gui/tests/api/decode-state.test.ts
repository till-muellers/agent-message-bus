import { describe, expect, test } from "bun:test";
import { decodeSpaceDetail, decodeSpaceList } from "../../src/api/decode-state.ts";
import { spaceDetail, spaceList } from "../fixtures.ts";

const you = { kind: "admin", name: "admin", spaces: [] };

const listWire = {
	you,
	spaces: [
		{ name: "demo", leaders: ["lead", "lead2"], admins: ["ops"], members: 1, joined: false, invites: 1 },
		{ name: "other", leaders: ["lead"], admins: [], members: 0, joined: false, invites: 0 },
	],
	inbox: { waiting: 2 },
};

const detailWire = {
	you,
	overview: { ...spaceDetail.overview, you: { name: "admin", role: null } },
	invites: spaceDetail.invites,
	inbox: { waiting: 0 },
};

function invalid(): unknown {
	return expect.objectContaining({ code: "invalid_response" });
}

describe("decoding the space list", () => {
	test("the spaces with their counts and the waiting inbox count", () => {
		expect(decodeSpaceList(listWire)).toEqual(spaceList);
	});

	test("a space without an invites count has none (no leader rights)", () => {
		const [demo] = decodeSpaceList({ ...listWire, spaces: [{ ...listWire.spaces[0], invites: undefined }] }).spaces;
		expect(demo?.invites).toBeUndefined();
	});

	const broken: [string, unknown][] = [
		["spaces that are no list", { ...listWire, spaces: {} }],
		["a leader that is not text", { ...listWire, spaces: [{ ...listWire.spaces[0], leaders: [1] }] }],
		["a negative member count", { ...listWire, spaces: [{ ...listWire.spaces[0], members: -1 }] }],
		["a fractional invite count", { ...listWire, spaces: [{ ...listWire.spaces[0], invites: 1.5 }] }],
		["joined that is not a flag", { ...listWire, spaces: [{ ...listWire.spaces[0], joined: "no" }] }],
		["no inbox", { ...listWire, inbox: undefined }],
	];
	for (const [what, json] of broken) {
		test(`a list with ${what} is an invalid response`, () => {
			expect(() => decodeSpaceList(json)).toThrow(invalid());
		});
	}
});

describe("decoding the space detail", () => {
	test("participants, topics, blackboards, recent threads and open invites", () => {
		expect(decodeSpaceDetail(detailWire)).toEqual(spaceDetail);
	});

	test("a session without leader rights gets no invites", () => {
		expect(decodeSpaceDetail({ ...detailWire, invites: undefined }).invites).toBeUndefined();
	});

	const overview = detailWire.overview;
	const broken: [string, unknown][] = [
		["no overview", { ...detailWire, overview: undefined }],
		["an unknown role", { ...detailWire, overview: { ...overview, participants: [{ name: "x", role: "boss", connected: false }] } }],
		["a topic without creator", { ...detailWire, overview: { ...overview, topics: [{ name: "all", subscribed: false }] } }],
		["a thread id that is not a number", {
			...detailWire,
			overview: { ...overview, recentThreads: [{ ...overview.recentThreads[0], id: "7" }] },
		}],
		["an invite without id", { ...detailWire, invites: [{ name: "new", invitedBy: "lead", expiresAt: "x" }] }],
	];
	for (const [what, json] of broken) {
		test(`a detail with ${what} is an invalid response`, () => {
			expect(() => decodeSpaceDetail(json)).toThrow(invalid());
		});
	}
});
