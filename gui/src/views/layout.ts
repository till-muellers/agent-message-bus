import type { Identity } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import type { AppState, Route } from "../state/app-state.ts";
import { h, type VNode } from "../view/h.ts";
import { loginView } from "./login.ts";

export interface Intents {
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

function content(route: Route): VNode | null {
	if (route.name !== "not-found") return null;
	return h("p", null, "Page not found. ", h("a", { href: hrefOf({ name: "spaces" }) }, "Back to the spaces"));
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
			const main = h("main", null, content(state.route));
			return h("div", { key: "shell" }, header(session.identity, state.route, intents), main);
		}
	}
}
