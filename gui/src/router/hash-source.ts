import type { Action } from "../state/actions.ts";
import { parseRoute } from "./routes.ts";

/** Dispatches navigate on every hash change; main.ts starts from the hash of the first load. */
export function listenToHash(win: Window, dispatch: (action: Action) => void): void {
	win.addEventListener("hashchange", () => dispatch({ type: "navigate", route: parseRoute(win.location.hash) }));
}
