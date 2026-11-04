import { createAdminsApi } from "./api/admins.ts";
import { createHttp, type Http } from "./api/http.ts";
import { createMutations } from "./api/mutations.ts";
import { createReads } from "./api/reads.ts";
import { createSessionApi } from "./api/session.ts";
import { loadView } from "./poll/load-view.ts";
import { createPoller, type Poller } from "./poll/poller.ts";
import { listenToHash } from "./router/hash-source.ts";
import { parseRoute } from "./router/routes.ts";
import { createAdminIntents } from "./state/admin-intents.ts";
import type { AppState } from "./state/app-state.ts";
import { initialState, reducer } from "./state/reducer.ts";
import { createSessionIntents } from "./state/session-intents.ts";
import { createSpaceIntents } from "./state/space-intents.ts";
import { createStore, type Store } from "./state/store.ts";
import { mount } from "./view/mount.ts";
import { type Intents, render } from "./views/layout.ts";

/** Polls the current route every 5 s while the page is visible, and at once when it is shown again. */
function startPolling(store: Store, http: Http): Poller {
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
	return poller;
}

function intentsOf(store: Store, http: Http, poller: Poller): { intents: Intents; restore(): Promise<void> } {
	const deps = { dispatch: store.dispatch, refresh: () => poller.refreshNow() };
	const session = createSessionIntents(createSessionApi(http), store.dispatch);
	const admins = createAdminIntents(createAdminsApi(http), deps);
	const space = createSpaceIntents(createMutations(http), deps);
	const intents: Intents = {
		signIn: (token) => void session.signIn(token),
		signOut: () => void session.signOut(),
		mintAdmin: (name) => void admins.mint(name),
		revokeAdmin: (name) => void admins.revoke(name),
		invite: (s, name, task) => void space.invite(s, name, task),
		cancelInvite: (s, invite) => void space.cancelInvite(s, invite),
		post: (s, threadId, body) => void space.post(s, threadId, body),
		send: (s, topic, body, critical) => void space.send(s, topic, body, critical),
		ask: (confirm) => space.ask(confirm),
		confirmed: (confirm) => void space.confirmed(confirm),
		dismiss: () => space.dismiss(),
	};
	return { intents, restore: () => session.restore() };
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
	const poller = startPolling(store, http);
	const { intents, restore } = intentsOf(store, http, poller);
	const draw = (state: AppState): void => mount(root, render(state, intents));
	store.subscribe(draw);
	draw(store.getState());
	listenToHash(window, store.dispatch);
	void restore();
}

const root = document.getElementById("app");
if (root) boot(root);
