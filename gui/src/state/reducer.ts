import { sameRoute } from "../router/routes.ts";
import type { Action } from "./actions.ts";
import type { AppState, Route } from "./app-state.ts";

export function initialState(route: Route): AppState {
	return { session: { status: "unknown" }, route };
}

/** Pure: returns state itself when the action changes nothing, so the store notifies nobody. */
export function reducer(state: AppState, action: Action): AppState {
	switch (action.type) {
		case "session/restored":
			return { ...state, session: { status: "signed-in", identity: action.identity, csrf: action.csrf } };
		case "session/signed-out":
			return { ...state, session: { status: "signed-out", error: action.error } };
		case "navigate":
			return sameRoute(state.route, action.route) ? state : { ...state, route: action.route };
	}
}
