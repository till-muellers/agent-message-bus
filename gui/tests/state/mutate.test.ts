import { describe, expect, test } from "bun:test";
import { ApiError } from "../../src/api/api-error.ts";
import type { Action } from "../../src/state/actions.ts";
import { type MutationDeps, mutate } from "../../src/state/mutate.ts";

function recorder(): { actions: Action[]; refreshes: () => number; deps: MutationDeps } {
	const actions: Action[] = [];
	let refreshed = 0;
	return { actions, refreshes: () => refreshed, deps: { dispatch: (a: Action) => void actions.push(a), refresh: () => void refreshed++ } };
}

describe("running a mutation", () => {
	test("a success dispatches its outcome and refreshes the page", async () => {
		const r = recorder();
		await mutate(r.deps, Promise.resolve(3), (n) => ({ type: "notice/shown", message: `got ${n}`, alert: false }));
		expect(r.actions).toEqual([{ type: "notice/shown", message: "got 3", alert: false }]);
		expect(r.refreshes()).toBe(1);
	});

	test("a refusal shows the server's message and refreshes nothing", async () => {
		const r = recorder();
		await mutate(r.deps, Promise.reject(new ApiError(400, "invalid", "name must match")), () => ({ type: "notice/shown", message: "done", alert: false }));
		expect(r.actions).toEqual([{ type: "notice/shown", message: "name must match", alert: true }]);
		expect(r.refreshes()).toBe(0);
	});

	test("a 401 signs out with the reason", async () => {
		const r = recorder();
		await mutate(r.deps, Promise.reject(new ApiError(401, "unauthenticated", "invalid token")), () => ({ type: "notice/shown", message: "done", alert: false }));
		expect(r.actions).toEqual([{ type: "session/signed-out", error: "invalid token" }]);
	});
});
