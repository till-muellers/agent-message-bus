import { describe, expect, test } from "bun:test";
import type { Identity } from "../../src/api/types.ts";
import type { AppState, Route } from "../../src/state/app-state.ts";
import { render } from "../../src/views/layout.ts";
import { byTag, findAll, textOf } from "../tree.ts";

const intents = { signIn: (): void => {}, signOut: (): void => {} };

function signedIn(identity: Identity, route: Route = { name: "spaces" }): AppState {
	return { session: { status: "signed-in", identity, csrf: "c1" }, route };
}

function navHrefs(state: AppState): (string | undefined)[] {
	const [nav] = findAll(render(state, intents), byTag("nav"));
	return nav ? findAll(nav, byTag("a")).map((a) => a.attrs.href) : [];
}

describe("the layout", () => {
	test("an unknown session shows neither the login nor the app", () => {
		const view = render({ session: { status: "unknown" }, route: { name: "spaces" } }, intents);
		expect(findAll(view, byTag("form"))).toEqual([]);
		expect(findAll(view, byTag("nav"))).toEqual([]);
	});

	test("a signed-out session shows the login", () => {
		const view = render({ session: { status: "signed-out" }, route: { name: "spaces" } }, intents);
		expect(findAll(view, byTag("form"))).toHaveLength(1);
	});

	test("the signed-in admin is named by kind and name", () => {
		const [header] = findAll(render(signedIn({ kind: "admin", name: "ops" }), intents), byTag("header"));
		expect(header && textOf(header)).toContain("Signed in as admin ops");
	});

	test("a member is named with its space", () => {
		const [header] = findAll(render(signedIn({ kind: "member", name: "worker", space: "demo" }), intents), byTag("header"));
		expect(header && textOf(header)).toContain("Signed in as member worker in space demo");
	});

	test("an admin gets the admin area in the navigation", () => {
		expect(navHrefs(signedIn({ kind: "admin", name: "ops" }))).toEqual(["#/", "#/inbox", "#/admins"]);
	});

	test("leaders and members get no admin area", () => {
		expect(navHrefs(signedIn({ kind: "leader", name: "lead" }))).toEqual(["#/", "#/inbox"]);
		expect(navHrefs(signedIn({ kind: "member", name: "worker", space: "demo" }))).toEqual(["#/", "#/inbox"]);
	});

	const currentCases: [Route, string[]][] = [
		[{ name: "inbox" }, ["#/inbox"]],
		[{ name: "spaces" }, ["#/"]],
		[{ name: "thread", space: "demo", threadId: 1 }, ["#/"]],
		[{ name: "not-found" }, []],
	];
	for (const [route, marked] of currentCases) {
		test(`the ${route.name} page marks ${marked.join(", ") || "nothing"} as current in the navigation`, () => {
			const [nav] = findAll(render(signedIn({ kind: "admin", name: "ops" }, route), intents), byTag("nav"));
			const current = nav ? findAll(nav, (n) => n.attrs["aria-current"] === "page") : [];
			expect(current.map((a) => a.attrs.href)).toEqual(marked);
		});
	}

	test("the sign-out button calls the sign-out intent", () => {
		let signedOut = 0;
		const view = render(signedIn({ kind: "admin", name: "ops" }), { ...intents, signOut: () => signedOut++ });
		const [button] = findAll(view, (n) => n.tag === "button" && textOf(n) === "Sign out");
		button?.on.click?.(new Event("click"));
		expect(signedOut).toBe(1);
	});

	test("an unknown page says so and links to the space list", () => {
		const view = render(signedIn({ kind: "admin", name: "ops" }, { name: "not-found" }), intents);
		const [main] = findAll(view, byTag("main"));
		expect(main && textOf(main)).toContain("Page not found");
		expect(main ? findAll(main, byTag("a")).map((a) => a.attrs.href) : []).toEqual(["#/"]);
	});
});
