import type { Action } from "../state/actions.ts";
import { parseRoute } from "./routes.ts";

/** Dispatches navigate for the current hash and for every hash change. */
export function listenToHash(win: Window, dispatch: (action: Action) => void): void {
	const navigate = (): void => dispatch({ type: "navigate", route: parseRoute(win.location.hash) });
	win.addEventListener("hashchange", navigate);
	navigate();
}
