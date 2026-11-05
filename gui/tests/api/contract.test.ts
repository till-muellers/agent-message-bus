import { describe, expect, test } from "bun:test";
import { decodeAdmins, decodeMinted } from "../../src/api/decode-admins.ts";
import { decodeEntries } from "../../src/api/decode-inbox.ts";
import { decodeCancelled, decodeInviteCode, decodePostId } from "../../src/api/decode-mutations.ts";
import { decodeSpaceDetail, decodeSpaceList } from "../../src/api/decode-state.ts";
import { decodeThread, decodeThreadInfos } from "../../src/api/decode-threads.ts";
import { decodeDeleted, decodeOk, decodeSession } from "../../src/api/decode.ts";
import { at, without, wire, wireNames } from "./wire.ts";

// Every fixture the Go handlers wrote decodes with the production decoder and keeps every
// field the GUI reads. Not decoded on purpose: "you" (the session names the operator) and
// the receipts of a post or a send.

type Expected = (json: unknown) => unknown;

const same: Expected = (json) => json;

function detail(json: unknown): unknown {
	return { overview: without(at(json, "overview"), "you"), invites: at(json, "invites"), inboxWaiting: at(json, "inbox", "waiting") };
}

const cases: Record<string, [(json: unknown) => unknown, Expected]> = {
	"state-list.json": [decodeSpaceList, (json) => ({ spaces: at(json, "spaces"), inboxWaiting: at(json, "inbox", "waiting") })],
	"state-space-leader.json": [decodeSpaceDetail, detail],
	"state-space-member.json": [decodeSpaceDetail, detail],
	"threads.json": [decodeThreadInfos, same],
	"thread.json": [decodeThread, same],
	"inbox.json": [decodeEntries, same],
	"admins.json": [decodeAdmins, same],
	"mint-admin.json": [decodeMinted, same],
	"revoke-admin.json": [decodeDeleted, (json) => at(json, "deleted")],
	"session.json": [decodeSession, (json) => ({ identity: without(json, "csrf"), csrf: at(json, "csrf") })],
	"sign-out.json": [decodeOk, () => true],
	"invite.json": [decodeInviteCode, (json) => ({ ...without(json, "expires_at"), expiresAt: at(json, "expires_at") })],
	"cancel-invite.json": [decodeCancelled, (json) => at(json, "cancelled")],
	"revoke.json": [decodeDeleted, (json) => at(json, "deleted")],
	"post.json": [decodePostId, (json) => at(json, "postId")],
	"send.json": [decodeOk, () => true],
	"delete-space.json": [decodeOk, () => true],
};

describe("the /web wire fixtures of the Go handlers", () => {
	test("every fixture has a decoder here", () => {
		expect(wireNames()).toEqual(Object.keys(cases).sort());
	});

	for (const [name, [decode, expected]] of Object.entries(cases)) {
		test(`${name} decodes without losing a field`, () => {
			const json = wire(name);
			expect(decode(json)).toEqual(expected(json));
		});
	}

	test("a space detail without leader rights carries no invites", () => {
		expect(decodeSpaceDetail(wire("state-space-member.json")).invites).toBeUndefined();
	});
});
