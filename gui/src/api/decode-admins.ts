import { count, type Decoder, fields, flag, list, listOf, text, textItem } from "./decode.ts";
import type { AdminInfo, MintedAdmin } from "./types.ts";

// Decoders of the admin area (contracts.md §10.3).

function admin(json: unknown, what: string): AdminInfo {
	const o = fields(json, what);
	return {
		name: text(o, "name", what),
		bootstrap: flag(o, "bootstrap", what),
		tokens: count(o, "tokens", what),
		spaces: list(o, "spaces", what, textItem),
	};
}

export const decodeAdmins: Decoder<AdminInfo[]> = (json) => listOf(json, "admins", admin);

export const decodeMinted: Decoder<MintedAdmin> = (json) => {
	const o = fields(json, "minted");
	return { name: text(o, "name", "minted"), token: text(o, "token", "minted") };
};
