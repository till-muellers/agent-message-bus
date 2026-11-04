import { toApiError } from "../api/api-error.ts";
import { sameRoute } from "../router/routes.ts";
import type { AppState, Route } from "../state/app-state.ts";
import type { Store } from "../state/store.ts";
import type { ViewData } from "../state/view-data.ts";

export interface Clock {
	setTimeout(fn: () => void, ms: number): number;
	clearTimeout(id: number): void;
	now(): number;
}

export interface PollerDeps {
	load(route: Route): Promise<ViewData> | undefined;
	store: Store;
	clock: Clock;
	visible(): boolean;
	intervalMs?: number;
}

export interface Poller {
	start(): void;
	stop(): void;
	/** Loads the current route now and restarts the interval (navigation, a successful mutation, the tab shown again). */
	refreshNow(): void;
}

/** Delay in intervals by failures in a row: 5 s while fine, then 5 → 10 → 30 s at the default interval. */
const BACKOFF = [1, 1, 2, 6] as const;

type Outcome = { data: ViewData } | { error: unknown };

class RoutePoller implements Poller {
	private timer: number | undefined;
	private generation = 0;
	private failures = 0;
	private watched: { route: Route; signedIn: boolean } | undefined;
	private unsubscribe: (() => void) | undefined;

	constructor(
		private readonly deps: PollerDeps,
		private readonly interval: number,
	) {}

	start(): void {
		this.unsubscribe = this.deps.store.subscribe((state) => this.onChange(state));
		this.onChange(this.deps.store.getState());
	}

	stop(): void {
		this.unsubscribe?.();
		this.halt();
	}

	refreshNow(): void {
		this.halt();
		const { session, route, view } = this.deps.store.getState();
		if (session.status !== "signed-in") return;
		if (!this.deps.visible()) return this.schedule();
		const request = this.deps.load(route);
		if (!request) return;
		const mine = this.generation;
		if (view.status === "idle") this.deps.store.dispatch({ type: "view/loading" });
		request.then(
			(data) => this.settled(route, mine, { data }),
			(error: unknown) => this.settled(route, mine, { error }),
		);
	}

	/** Polls from sign-in on, restarts at once on navigation, stops at sign-out. */
	private onChange(state: AppState): void {
		const signedIn = state.session.status === "signed-in";
		const changed = !this.watched || this.watched.signedIn !== signedIn || !sameRoute(this.watched.route, state.route);
		this.watched = { route: state.route, signedIn };
		if (!changed) return;
		if (signedIn) this.refreshNow();
		else this.halt();
	}

	/** Answers of a superseded load (navigation, refreshNow, stop) are dropped. */
	private settled(route: Route, mine: number, outcome: Outcome): void {
		if (mine !== this.generation) return;
		const { store, clock } = this.deps;
		if ("data" in outcome) {
			this.failures = 0;
			store.dispatch({ type: "view/loaded", route, data: outcome.data, at: clock.now() });
		} else {
			this.failures++;
			store.dispatch({ type: "view/failed", route, error: toApiError(outcome.error) });
		}
		if (store.getState().session.status === "signed-in") this.schedule();
	}

	private schedule(): void {
		this.cancel();
		const delay = this.interval * (BACKOFF[Math.min(this.failures, BACKOFF.length - 1)] ?? 1);
		this.timer = this.deps.clock.setTimeout(() => this.refreshNow(), delay);
	}

	private cancel(): void {
		if (this.timer !== undefined) this.deps.clock.clearTimeout(this.timer);
		this.timer = undefined;
	}

	private halt(): void {
		this.cancel();
		this.generation++;
	}
}

export function createPoller(deps: PollerDeps): Poller {
	return new RoutePoller(deps, deps.intervalMs ?? 5_000);
}
