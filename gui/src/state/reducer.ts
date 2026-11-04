import type { ApiError } from "../api/api-error.ts";
import { sameRoute } from "../router/routes.ts";
import type { Action } from "./actions.ts";
import type { AppState, Resource, Route } from "./app-state.ts";
import { sameData } from "./same-data.ts";
import type { ViewData } from "./view-data.ts";

export function initialState(route: Route): AppState {
	return { session: { status: "unknown" }, route, view: { status: "idle" } };
}

function lastData(view: Resource<ViewData>): ViewData | undefined {
	switch (view.status) {
		case "ready":
			return view.data;
		case "loading":
		case "error":
			return view.last;
		case "idle":
			return undefined;
	}
}

function loaded(state: AppState, data: ViewData, at: number): AppState {
	if (state.view.status === "ready" && sameData(state.view.data, data)) return state;
	return { ...state, view: { status: "ready", data, at } };
}

/**
 * Any 401 means the session is gone (expired, revoked, rotated token): back to the login with the reason.
 * A transient failure (unreachable, rate limit, server error) keeps the last data; a 403 or 404 drops it.
 */
function failed(state: AppState, error: ApiError): AppState {
	if (error.status === 401) {
		return { session: { status: "signed-out", error: error.message }, route: state.route, view: { status: "idle" } };
	}
	const transient = error.status === 0 || error.status === 429 || error.status >= 500;
	return { ...state, view: { status: "error", error, last: transient ? lastData(state.view) : undefined } };
}

/** Pure: returns state itself when the action changes nothing, so the store notifies nobody. */
export function reducer(state: AppState, action: Action): AppState {
	switch (action.type) {
		case "session/restored":
			return { ...state, session: { status: "signed-in", identity: action.identity, csrf: action.csrf } };
		case "session/signed-out":
			return { session: { status: "signed-out", error: action.error }, route: state.route, view: { status: "idle" } };
		case "navigate":
			return sameRoute(state.route, action.route) ? state : { session: state.session, route: action.route, view: { status: "idle" } };
		case "view/loading":
			return { ...state, view: { status: "loading", last: lastData(state.view) } };
		case "view/loaded":
			return sameRoute(state.route, action.route) ? loaded(state, action.data, action.at) : state;
		case "view/failed":
			return sameRoute(state.route, action.route) ? failed(state, action.error) : state;
		case "secret/shown":
			return { ...state, shownOnce: { label: action.label, secret: action.secret }, notice: undefined };
		case "notice/shown":
			return { ...state, notice: { message: action.message, alert: action.alert }, shownOnce: undefined };
	}
}
