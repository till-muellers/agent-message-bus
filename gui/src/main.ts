import { createHttp } from "./api/http.ts";
import { createSessionApi } from "./api/session.ts";
import { listenToHash } from "./router/hash-source.ts";
import { parseRoute } from "./router/routes.ts";
import type { AppState } from "./state/app-state.ts";
import { initialState, reducer } from "./state/reducer.ts";
import { createSessionIntents } from "./state/session-intents.ts";
import { createStore } from "./state/store.ts";
import { mount } from "./view/mount.ts";
import { type Intents, render } from "./views/layout.ts";

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
	void session.restore();
}

const root = document.getElementById("app");
if (root) boot(root);
