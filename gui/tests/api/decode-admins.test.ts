import { describe, expect, test } from "bun:test";
import { decodeAdmins, decodeMinted } from "../../src/api/decode-admins.ts";
import { decodeDeleted } from "../../src/api/decode.ts";
import { admins } from "../fixtures.ts";

function invalid(): unknown {
	return expect.objectContaining({ code: "invalid_response" });
}

describe("decoding the admin area", () => {
	test("the admin list with bootstrap flag, token count and joined spaces", () => {
		expect(decodeAdmins(admins)).toEqual(admins);
	});

	test("an admin without a token count is refused", () => {
		expect(() => decodeAdmins([{ name: "ops", bootstrap: false, spaces: [] }])).toThrow(invalid());
	});

	test("a minted admin carries its name and token", () => {
		expect(decodeMinted({ name: "ops", token: "amb_admin_x" })).toEqual({ name: "ops", token: "amb_admin_x" });
		expect(() => decodeMinted({ name: "ops" })).toThrow(invalid());
	});

	test("a revoke reports whether anything was removed", () => {
		expect(decodeDeleted({ deleted: false })).toBe(false);
		expect(() => decodeDeleted({})).toThrow(invalid());
	});
});
