import { describe, expect, test } from "bun:test";
import { initialState, reducer } from "../../src/state/reducer.ts";

const ops = { kind: "admin", name: "ops" } as const;

describe("the reducer", () => {
	test("the session is unknown until it was restored or refused", () => {
		expect(initialState({ name: "inbox" })).toEqual({ session: { status: "unknown" }, route: { name: "inbox" } });
	});

	test("a restored session is signed in with its identity and CSRF token", () => {
		const next = reducer(initialState({ name: "spaces" }), { type: "session/restored", identity: ops, csrf: "c1" });
		expect(next.session).toEqual({ status: "signed-in", identity: ops, csrf: "c1" });
	});

	test("signing out forgets the identity and the CSRF token and keeps the error to show", () => {
		const signedIn = reducer(initialState({ name: "spaces" }), { type: "session/restored", identity: ops, csrf: "c1" });
		const next = reducer(signedIn, { type: "session/signed-out", error: "invalid token" });
		expect(next.session).toEqual({ status: "signed-out", error: "invalid token" });
	});

	test("navigating changes the route", () => {
		const next = reducer(initialState({ name: "spaces" }), { type: "navigate", route: { name: "inbox" } });
		expect(next.route).toEqual({ name: "inbox" });
	});

	test("navigating to the current route returns the same state", () => {
		const state = initialState({ name: "space", space: "demo" });
		expect(reducer(state, { type: "navigate", route: { name: "space", space: "demo" } })).toBe(state);
	});
});
