import { describe, expect, test } from "bun:test";
import { decodeAdmins, decodeMinted } from "../../src/api/decode-admins.ts";
import { decodeEntries } from "../../src/api/decode-inbox.ts";
import { decodeCancelled, decodeInviteCode, decodePostId } from "../../src/api/decode-mutations.ts";
import { decodeSpaceDetail, decodeSpaceList } from "../../src/api/decode-state.ts";
import { decodeThread, decodeThreadInfos } from "../../src/api/decode-threads.ts";
import { type Decoder, decodeDeleted, decodeOk, decodeSession } from "../../src/api/decode.ts";
import type { SpaceDetail } from "../../src/api/types.ts";
import { at, without, wire, wireNames } from "./wire.ts";

// Every fixture the Go handlers wrote decodes with the production decoder, and the decoded
// value, written back in wire shape, equals the fixture: a renamed, added or dropped field
// fails. Left out on purpose: "you" (the session names the operator) and the receipts of a
// post or a send.

/** The decoded value written back in wire shape, and the fixture without the fields left out. */
type Check = (json: unknown) => { decoded: unknown; sent: unknown };

function check<T>(decode: Decoder<T>, back: (value: T) => unknown, kept: (json: unknown) => unknown = (json) => json): Check {
	return (json) => ({ decoded: back(decode(json)), sent: kept(json) });
}

const asSent = <T>(value: T): T => value;

const detail = check(
	decodeSpaceDetail,
	(d: SpaceDetail) => ({ overview: d.overview, invites: d.invites, inbox: { waiting: d.inboxWaiting } }),
	(json) => ({ ...without(json, "you"), overview: without(at(json, "overview"), "you") }),
);

const cases: Record<string, Check> = {
	"state-list.json": check(decodeSpaceList, (l) => ({ spaces: l.spaces, inbox: { waiting: l.inboxWaiting } }), (json) => without(json, "you")),
	"state-space-leader.json": detail,
	"state-space-member.json": detail,
	"threads.json": check(decodeThreadInfos, asSent),
	"thread.json": check(decodeThread, asSent),
	"inbox.json": check(decodeEntries, asSent),
	"admins.json": check(decodeAdmins, asSent),
	"mint-admin.json": check(decodeMinted, asSent),
	"revoke-admin.json": check(decodeDeleted, (deleted) => ({ deleted })),
	"session.json": check(decodeSession, (s) => ({ ...s.identity, csrf: s.csrf })),
	"sign-out.json": check(decodeOk, (ok) => ({ ok })),
	"invite.json": check(decodeInviteCode, ({ expiresAt, ...code }) => ({ ...code, expires_at: expiresAt })),
	"cancel-invite.json": check(decodeCancelled, (cancelled) => ({ cancelled })),
	"revoke.json": check(decodeDeleted, (deleted) => ({ deleted })),
	"post.json": check(decodePostId, (postId) => ({ postId }), (json) => without(json, "receipts")),
	"send.json": check(decodeOk, (ok) => ({ ok }), (json) => without(json, "receipts")),
	"delete-space.json": check(decodeOk, (ok) => ({ ok })),
};

describe("the /web wire fixtures of the Go handlers", () => {
	test("every fixture has a decoder here", () => {
		expect(wireNames()).toEqual(Object.keys(cases).sort());
	});

	for (const [name, contract] of Object.entries(cases)) {
		test(`${name} decodes without losing or renaming a field`, () => {
			const { decoded, sent } = contract(wire(name));
			expect(decoded).toEqual(sent);
		});
	}

	test("a space detail without leader rights carries no invites", () => {
		expect(decodeSpaceDetail(wire("state-space-member.json")).invites).toBeUndefined();
	});
});
