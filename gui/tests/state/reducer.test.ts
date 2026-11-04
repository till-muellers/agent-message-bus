import { describe, expect, test } from "bun:test";
import { ApiError } from "../../src/api/api-error.ts";
import type { AppState } from "../../src/state/app-state.ts";
import { initialState, reducer } from "../../src/state/reducer.ts";
import type { ViewData } from "../../src/state/view-data.ts";
import { spaceList } from "../fixtures.ts";

const ops = { kind: "admin", name: "ops" } as const;
const listData: ViewData = { kind: "spaces", list: spaceList };

function signedIn(): AppState {
	return reducer(initialState({ name: "spaces" }), { type: "session/restored", identity: ops, csrf: "c1" });
}

function ready(data: ViewData = listData): AppState {
	return reducer(signedIn(), { type: "view/loaded", route: { name: "spaces" }, data, at: 1 });
}

describe("the reducer: session and route", () => {
	test("the session is unknown and no view is loaded at first", () => {
		expect(initialState({ name: "inbox" })).toEqual({
			session: { status: "unknown" },
			route: { name: "inbox" },
			view: { status: "idle" },
		});
	});

	test("a restored session is signed in with its identity and CSRF token", () => {
		expect(signedIn().session).toEqual({ status: "signed-in", identity: ops, csrf: "c1" });
	});

	test("signing out forgets identity, CSRF token and view data and keeps the error to show", () => {
		const next = reducer(ready(), { type: "session/signed-out", error: "invalid token" });
		expect(next.session).toEqual({ status: "signed-out", error: "invalid token" });
		expect(next.view).toEqual({ status: "idle" });
	});

	test("navigating changes the route and drops the data of the old one", () => {
		const next = reducer(ready(), { type: "navigate", route: { name: "inbox" } });
		expect(next.route).toEqual({ name: "inbox" });
		expect(next.view).toEqual({ status: "idle" });
	});

	test("navigating to the current route returns the same state", () => {
		const state = initialState({ name: "space", space: "demo" });
		expect(reducer(state, { type: "navigate", route: { name: "space", space: "demo" } })).toBe(state);
	});
});

describe("the reducer: view data", () => {
	test("loading keeps the last data to show meanwhile", () => {
		expect(reducer(ready(), { type: "view/loading" }).view).toEqual({ status: "loading", last: listData });
	});

	test("loaded data of the current route is ready", () => {
		expect(ready().view).toEqual({ status: "ready", data: listData, at: 1 });
	});

	test("data that equals the shown data returns the same state, so nothing re-renders", () => {
		const state = ready();
		const again = structuredClone(listData);
		expect(reducer(state, { type: "view/loaded", route: { name: "spaces" }, data: again, at: 2 })).toBe(state);
	});

	test("a late answer for a route the operator already left is dropped", () => {
		const state = reducer(signedIn(), { type: "navigate", route: { name: "inbox" } });
		expect(reducer(state, { type: "view/loaded", route: { name: "spaces" }, data: listData, at: 1 })).toBe(state);
		const failed = reducer(state, { type: "view/failed", route: { name: "spaces" }, error: new ApiError(500, "x", "y") });
		expect(failed).toBe(state);
	});

	test("a failure keeps the last data and the error to show", () => {
		const error = new ApiError(0, "network", "the server is unreachable");
		expect(reducer(ready(), { type: "view/failed", route: { name: "spaces" }, error }).view).toEqual({
			status: "error",
			error,
			last: listData,
		});
	});

	test("a 401 signs out with the server's message", () => {
		const error = new ApiError(401, "unauthenticated", "invalid token");
		const next = reducer(ready(), { type: "view/failed", route: { name: "spaces" }, error });
		expect(next.session).toEqual({ status: "signed-out", error: "invalid token" });
		expect(next.view).toEqual({ status: "idle" });
	});

	test("a 403 or 404 after the page was shown drops its data: the space is gone or out of reach", () => {
		const error = new ApiError(404, "not_found", "space demo not found");
		expect(reducer(ready(), { type: "view/failed", route: { name: "spaces" }, error }).view).toEqual({ status: "error", error });
	});
});

describe("the reducer: messages after a mutation", () => {
	test("a secret is shown and replaces a notice", () => {
		const noticed = reducer(ready(), { type: "notice/shown", message: "Admin ops revoked.", alert: false });
		const next = reducer(noticed, { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" });
		expect(next.shownOnce).toEqual({ label: "Token of admin ops", secret: "amb_admin_x" });
		expect(next.notice).toBeUndefined();
	});

	test("a notice replaces a shown secret", () => {
		const shown = reducer(ready(), { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" });
		const next = reducer(shown, { type: "notice/shown", message: "agent lead is a leader agent", alert: true });
		expect(next.notice).toEqual({ message: "agent lead is a leader agent", alert: true });
		expect(next.shownOnce).toBeUndefined();
	});

	test("navigating drops the secret and the notice", () => {
		const shown = reducer(ready(), { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" });
		const next = reducer(shown, { type: "navigate", route: { name: "inbox" } });
		expect(next.shownOnce).toBeUndefined();
		expect(next.notice).toBeUndefined();
	});

	test("signing out drops the secret", () => {
		const shown = reducer(ready(), { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" });
		expect(reducer(shown, { type: "session/signed-out" }).shownOnce).toBeUndefined();
	});

	test("a secret or notice arriving after sign-out is dropped", () => {
		const out = reducer(ready(), { type: "session/signed-out" });
		expect(reducer(out, { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" })).toBe(out);
		expect(reducer(out, { type: "notice/shown", message: "Admin ops revoked.", alert: false })).toBe(out);
	});

	test("a poll keeps the secret", () => {
		const shown = reducer(ready(), { type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" });
		const next = reducer(shown, { type: "view/loaded", route: { name: "spaces" }, data: { kind: "spaces", list: { ...spaceList, inboxWaiting: 9 } }, at: 2 });
		expect(next.shownOnce?.secret).toBe("amb_admin_x");
	});
});
