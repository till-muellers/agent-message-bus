import { describe, expect, test } from "bun:test";
import { decodeOk, decodeSession } from "../../src/api/decode.ts";

describe("decoding the session", () => {
	test("an admin session has kind, name and CSRF token", () => {
		expect(decodeSession({ kind: "admin", name: "ops", csrf: "c1" })).toEqual({
			identity: { kind: "admin", name: "ops" },
			csrf: "c1",
		});
	});

	test("a member session names its space", () => {
		expect(decodeSession({ kind: "member", name: "worker", space: "demo", csrf: "c1" }).identity).toEqual({
			kind: "member",
			name: "worker",
			space: "demo",
		});
	});

	const broken: [string, unknown][] = [
		["no object", "admin"],
		["an array", []],
		["no kind", { name: "ops", csrf: "c1" }],
		["an unknown kind", { kind: "root", name: "ops", csrf: "c1" }],
		["no name", { kind: "admin", csrf: "c1" }],
		["a name that is not text", { kind: "admin", name: 7, csrf: "c1" }],
		["no CSRF token", { kind: "admin", name: "ops" }],
		["a space that is not text", { kind: "member", name: "w", space: 1, csrf: "c1" }],
	];
	for (const [what, json] of broken) {
		test(`a session with ${what} is an invalid response`, () => {
			expect(() => decodeSession(json)).toThrow(expect.objectContaining({ code: "invalid_response" }));
		});
	}
});

describe("decoding ok", () => {
	test("{ok: true} passes", () => {
		expect(decodeOk({ ok: true })).toBe(true);
	});

	test("anything else is an invalid response", () => {
		expect(() => decodeOk({ ok: false })).toThrow(expect.objectContaining({ code: "invalid_response" }));
	});
});
