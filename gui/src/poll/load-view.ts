import type { Reads } from "../api/reads.ts";
import type { Route } from "../state/app-state.ts";
import type { ViewData } from "../state/view-data.ts";

/** The request behind a route's view, or undefined for a page without data. */
export function loadView(reads: Reads, route: Route): Promise<ViewData> | undefined {
	switch (route.name) {
		case "spaces":
			return reads.spaces().then((list) => ({ kind: "spaces", list }));
		case "space":
			return reads.space(route.space).then((detail) => ({ kind: "space", detail }));
		case "board": {
			const { space, board } = route;
			return reads.threads(space, board).then((threads) => ({ kind: "board", space, board, threads }));
		}
		case "thread":
			return reads.thread(route.space, route.threadId).then((thread) => ({ kind: "thread", space: route.space, thread }));
		case "inbox":
			return reads.inbox().then((read) => ({ kind: "inbox", ...read }));
		case "admins":
			return reads.admins().then((admins) => ({ kind: "admins", admins }));
		case "not-found":
			return undefined;
	}
}
