import { count, type Decoder, fields, flag, text } from "./decode.ts";
import type { InviteCode } from "./types.ts";

// Decoders of the mutation results (contracts.md §10.4, §10.6).

export const decodeInviteCode: Decoder<InviteCode> = (json) => {
	const o = fields(json, "invite");
	return {
		name: text(o, "name", "invite"),
		space: text(o, "space", "invite"),
		code: text(o, "code", "invite"),
		expiresAt: text(o, "expires_at", "invite"),
	};
};

/** {cancelled}: false for an id that is no longer pending (used, expired, cancelled, unknown). */
export const decodeCancelled: Decoder<boolean> = (json) => flag(fields(json, "result"), "cancelled", "result");

export const decodePostId: Decoder<number> = (json) => count(fields(json, "result"), "postId", "result");
