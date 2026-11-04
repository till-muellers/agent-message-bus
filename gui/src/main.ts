import { createHttp, type Http } from "./api/http.ts";
import { createReads } from "./api/reads.ts";
import { createSessionApi } from "./api/session.ts";
import { loadView } from "./poll/load-view.ts";
import { createPoller } from "./poll/poller.ts";
import { listenToHash } from "./router/hash-source.ts";
import { parseRoute } from "./router/routes.ts";
import type { AppState } from "./state/app-state.ts";
import { initialState, reducer } from "./state/reducer.ts";
import { createSessionIntents } from "./state/session-intents.ts";
import { createStore, type Store } from "./state/store.ts";
import { mount } from "./view/mount.ts";
import { type Intents, render } from "./views/layout.ts";

/** Polls the current route every 5 s while the page is visible, and at once when it is shown again. */
function startPolling(store: Store, http: Http): void {
	const reads = createReads(http);
	const poller = createPoller({
		load: (route) => loadView(reads, route),
		store,
		clock: { setTimeout: (fn, ms) => window.setTimeout(fn, ms), clearTimeout: (id) => window.clearTimeout(id), now: Date.now },
		visible: () => document.visibilityState === "visible",
	});
	document.addEventListener("visibilitychange", () => {
		if (document.visibilityState === "visible") poller.refreshNow();
	});
	poller.start();
}

function boot(root: HTMLElement): void {
	const store = createStore(reducer, initialState(parseRoute(location.hash)));
	const http = createHttp({
		fetchFn: (path, init) => fetch(path, init),
		csrf: () => {
			const { session } = store.getState();
			return session.status === "signed-in" ? session.csrf : undefined;
		},
	});
	const session = createSessionIntents(createSessionApi(http), store.dispatch);
	const intents: Intents = { signIn: (token) => void session.signIn(token), signOut: () => void session.signOut() };
	const draw = (state: AppState): void => mount(root, render(state, intents));
	store.subscribe(draw);
	draw(store.getState());
	listenToHash(window, store.dispatch);
	startPolling(store, http);
	void session.restore();
}

const root = document.getElementById("app");
if (root) boot(root);
