import { describe, expect, test } from "bun:test";
import type { AppState } from "../../src/state/app-state.ts";
import { initialState, reducer } from "../../src/state/reducer.ts";
import { createStore } from "../../src/state/store.ts";

describe("the store", () => {
	test("dispatch applies the reducer and notifies subscribers", () => {
		const store = createStore(reducer, initialState({ name: "spaces" }));
		const seen: AppState[] = [];
		store.subscribe((state) => seen.push(state));
		store.dispatch({ type: "navigate", route: { name: "inbox" } });
		expect(store.getState().route).toEqual({ name: "inbox" });
		expect(seen).toEqual([store.getState()]);
	});

	test("an action that changes nothing notifies nobody", () => {
		const store = createStore(reducer, initialState({ name: "spaces" }));
		let calls = 0;
		store.subscribe(() => calls++);
		store.dispatch({ type: "navigate", route: { name: "spaces" } });
		expect(calls).toBe(0);
	});

	test("an unsubscribed listener is not called again", () => {
		const store = createStore(reducer, initialState({ name: "spaces" }));
		let calls = 0;
		const unsubscribe = store.subscribe(() => calls++);
		unsubscribe();
		store.dispatch({ type: "navigate", route: { name: "inbox" } });
		expect(calls).toBe(0);
	});
});
