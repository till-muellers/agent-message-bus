import { describe, expect, test } from "bun:test";
import { ApiError } from "../../src/api/api-error.ts";
import { type Clock, createPoller } from "../../src/poll/poller.ts";
import type { Route } from "../../src/state/app-state.ts";
import type { ViewData } from "../../src/state/view-data.ts";
import { initialState, reducer } from "../../src/state/reducer.ts";
import { createStore, type Store } from "../../src/state/store.ts";
import { spaceList } from "../fixtures.ts";

class FakeClock implements Clock {
	private timers = new Map<number, { at: number; fn: () => void }>();
	private nextId = 1;
	time = 0;

	setTimeout(fn: () => void, ms: number): number {
		const id = this.nextId++;
		this.timers.set(id, { at: this.time + ms, fn });
		return id;
	}

	clearTimeout(id: number): void {
		this.timers.delete(id);
	}

	now(): number {
		return this.time;
	}

	/** The delays of the pending timers, from now. */
	pending(): number[] {
		return [...this.timers.values()].map((t) => t.at - this.time);
	}

	async advance(ms: number): Promise<void> {
		this.time += ms;
		for (const [id, timer] of [...this.timers]) {
			if (timer.at > this.time) continue;
			this.timers.delete(id);
			timer.fn();
		}
		await settle();
	}
}

async function settle(): Promise<void> {
	for (let i = 0; i < 5; i++) await Promise.resolve();
}

interface Rig {
	store: Store;
	clock: FakeClock;
	loads: Route[];
	outcomes: (ViewData | ApiError)[];
	visible: boolean;
}

function rig(route: Route = { name: "spaces" }): Rig & { poller: ReturnType<typeof createPoller> } {
	const r: Rig = { store: createStore(reducer, initialState(route)), clock: new FakeClock(), loads: [], outcomes: [], visible: true };
	const poller = createPoller({
		store: r.store,
		clock: r.clock,
		visible: () => r.visible,
		load: (loaded) => {
			if (loaded.name === "not-found") return undefined;
			r.loads.push(loaded);
			const next = r.outcomes.shift() ?? { kind: "spaces", list: spaceList };
			return next instanceof ApiError ? Promise.reject(next) : Promise.resolve(next);
		},
	});
	return { ...r, poller, get visible() { return r.visible; }, set visible(v) { r.visible = v; } };
}

function signIn(store: Store): void {
	store.dispatch({ type: "session/restored", identity: { kind: "admin", name: "admin" }, csrf: "c1" });
}

describe("the poller", () => {
	test("loads nothing before the session is signed in", async () => {
		const { poller, loads, clock } = rig();
		poller.start();
		await clock.advance(20_000);
		expect(loads).toEqual([]);
	});

	test("loads the current route at once after sign-in and then every 5 s", async () => {
		const { poller, loads, store, clock } = rig();
		poller.start();
		signIn(store);
		await settle();
		expect(loads).toHaveLength(1);
		expect(store.getState().view).toMatchObject({ status: "ready", data: { kind: "spaces" } });
		await clock.advance(5_000);
		expect(loads).toHaveLength(2);
	});

	test("loads at once on navigation, not after the rest of the interval", async () => {
		const { poller, loads, store, clock } = rig();
		poller.start();
		signIn(store);
		await settle();
		store.dispatch({ type: "navigate", route: { name: "space", space: "demo" } });
		await settle();
		expect(loads.at(-1)).toEqual({ name: "space", space: "demo" });
		expect(clock.pending()).toEqual([5_000]);
	});

	test("skips the load while the page is hidden", async () => {
		const r = rig();
		r.poller.start();
		signIn(r.store);
		await settle();
		r.visible = false;
		await r.clock.advance(5_000);
		expect(r.loads).toHaveLength(1);
		r.visible = true;
		await r.clock.advance(5_000);
		expect(r.loads).toHaveLength(2);
	});

	test("backs off 5, 10, then 30 s while loads fail", async () => {
		const r = rig();
		const down = new ApiError(0, "network", "the server is unreachable");
		r.outcomes.push(down, down, down, down);
		r.poller.start();
		signIn(r.store);
		await settle();
		expect(r.clock.pending()).toEqual([5_000]);
		await r.clock.advance(5_000);
		expect(r.clock.pending()).toEqual([10_000]);
		await r.clock.advance(10_000);
		expect(r.clock.pending()).toEqual([30_000]);
		await r.clock.advance(30_000);
		expect(r.clock.pending()).toEqual([30_000]);
		expect(r.store.getState().view).toMatchObject({ status: "error", error: { code: "network" } });
	});

	test("a 401 signs out and stops polling", async () => {
		const r = rig();
		r.outcomes.push(new ApiError(401, "unauthenticated", "invalid token"));
		r.poller.start();
		signIn(r.store);
		await settle();
		expect(r.store.getState().session).toEqual({ status: "signed-out", error: "invalid token" });
		expect(r.clock.pending()).toEqual([]);
	});

	test("refreshNow loads at once and restarts the interval", async () => {
		const { poller, loads, store, clock } = rig();
		poller.start();
		signIn(store);
		await settle();
		await clock.advance(3_000);
		poller.refreshNow();
		await settle();
		expect(loads).toHaveLength(2);
		expect(clock.pending()).toEqual([5_000]);
	});

	test("a route without data loads nothing and schedules nothing", async () => {
		const { poller, loads, store, clock } = rig({ name: "not-found" });
		poller.start();
		signIn(store);
		await settle();
		expect(loads).toEqual([]);
		expect(clock.pending()).toEqual([]);
	});

	test("stop cancels the next load", async () => {
		const { poller, loads, store, clock } = rig();
		poller.start();
		signIn(store);
		await settle();
		poller.stop();
		await clock.advance(30_000);
		expect(loads).toHaveLength(1);
	});
});
