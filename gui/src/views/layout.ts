import type { Identity } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import type { AppState, Route } from "../state/app-state.ts";
import type { ViewData } from "../state/view-data.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { type AdminAreaIntents, adminsView } from "./admins.ts";
import { boardView } from "./board.ts";
import { type ConfirmIntents, confirmView } from "./confirm.ts";
import type { FormIntents } from "./forms.ts";
import { inboxView } from "./inbox.ts";
import { loginView } from "./login.ts";
import { shownOnceView } from "./shown-once.ts";
import { type SpaceViewIntents, spaceView } from "./space.ts";
import { spacesView } from "./spaces.ts";
import { threadView } from "./thread.ts";

export interface Intents extends AdminAreaIntents, SpaceViewIntents, FormIntents, ConfirmIntents {
	signIn(token: string): void;
	signOut(): void;
}

const NAV: readonly { route: Route; label: string; adminOnly: boolean }[] = [
	{ route: { name: "spaces" }, label: "Spaces", adminOnly: false },
	{ route: { name: "inbox" }, label: "Inbox", adminOnly: false },
	{ route: { name: "admins" }, label: "Admins", adminOnly: true },
];

function signedInAs(identity: Identity): string {
	const who = `Signed in as ${identity.kind} ${identity.name}`;
	return identity.space === undefined ? who : `${who} in space ${identity.space}`;
}

/** The navigation entry a route belongs to: a space, board or thread is under the space list. */
function sectionOf(route: Route): Route["name"] | undefined {
	switch (route.name) {
		case "inbox":
		case "admins":
		case "spaces":
			return route.name;
		case "space":
		case "board":
		case "thread":
			return "spaces";
		case "not-found":
			return undefined;
	}
}

function nav(identity: Identity, current: Route): VNode {
	const section = sectionOf(current);
	const links = NAV.filter((item) => !item.adminOnly || identity.kind === "admin").map((item) => {
		const ariaCurrent = item.route.name === section ? "page" : undefined;
		return h("a", { key: item.route.name, href: hrefOf(item.route), "aria-current": ariaCurrent }, item.label);
	});
	return h("nav", null, links);
}

function header(identity: Identity, route: Route, intents: Intents): VNode {
	return h(
		"header",
		null,
		h("p", null, signedInAs(identity)),
		nav(identity, route),
		h("button", { type: "button", on: { click: () => intents.signOut() } }, "Sign out"),
	);
}

/** The view of route for data; null when the data belongs to another kind of page. */
function viewOf(route: Route, data: ViewData | undefined, identity: Identity, intents: Intents): VNode | null {
	if (route.name !== data?.kind) return null;
	switch (data.kind) {
		case "spaces":
			return spacesView(data.list);
		case "space":
			return spaceView(data.detail, intents, identity.kind === "admin");
		case "board":
			return boardView(data.space, data.board, data.threads);
		case "thread":
			return threadView(data.space, data.thread, intents);
		case "inbox":
			return inboxView(identity, data.entries);
		case "admins":
			return adminsView(data.admins, intents);
	}
}

/** The main area: the route's data, "Loading…" before the first answer, the server's message on failure. */
function content({ route, view }: AppState, identity: Identity, intents: Intents): ChildArg[] {
	if (route.name === "not-found") {
		return [h("p", null, "Page not found. ", h("a", { href: hrefOf({ name: "spaces" }) }, "Back to the spaces"))];
	}
	const page = (data: ViewData | undefined): VNode | null => viewOf(route, data, identity, intents);
	switch (view.status) {
		case "idle":
			return [];
		case "loading":
			return [view.last ? page(view.last) : h("p", { key: "loading" }, "Loading…")];
		case "ready":
			return [page(view.data)];
		case "error":
			return [h("p", { key: "error", role: "alert" }, view.error.message), page(view.last)];
	}
}

/** An open confirmation, else the outcome of the last mutation: a secret shown once, or the notice (an alert when refused). */
function outcome({ confirm, shownOnce, notice }: AppState, intents: Intents): ChildArg {
	if (confirm) return confirmView(confirm, intents);
	if (shownOnce) return shownOnceView(shownOnce);
	if (!notice) return null;
	return h("p", { key: "notice", role: notice.alert ? "alert" : "status" }, notice.message);
}

/** The whole page for state: the login, or the signed-in shell with the current route's view. */
export function render(state: AppState, intents: Intents): VNode {
	const { session } = state;
	switch (session.status) {
		case "unknown":
			return h("main", { key: "restoring" });
		case "signed-out":
			return loginView(session.error, intents.signIn);
		case "signed-in": {
			const main = h("main", null, outcome(state, intents), content(state, session.identity, intents));
			return h("div", { key: "shell" }, header(session.identity, state.route, intents), main);
		}
	}
}
