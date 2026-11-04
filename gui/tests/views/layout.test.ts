import { describe, expect, test } from "bun:test";
import type { Identity } from "../../src/api/types.ts";
import { ApiError } from "../../src/api/api-error.ts";
import type { AppState, Route } from "../../src/state/app-state.ts";
import type { ViewData } from "../../src/state/view-data.ts";
import { render } from "../../src/views/layout.ts";
import { admins, entries, spaceDetail, spaceList, thread, threadInfos } from "../fixtures.ts";
import { byTag, findAll, textOf } from "../tree.ts";

const intents = { signIn: (): void => {}, signOut: (): void => {}, mintAdmin: (): void => {}, revokeAdmin: (): void => {} };

function signedIn(identity: Identity, route: Route = { name: "spaces" }): AppState {
	return { session: { status: "signed-in", identity, csrf: "c1" }, route, view: { status: "idle" } };
}

const admin: Identity = { kind: "admin", name: "admin" };
const listData: ViewData = { kind: "spaces", list: spaceList };

function mainOf(state: AppState): string {
	const [main] = findAll(render(state, intents), byTag("main"));
	return main ? textOf(main) : "";
}

function navHrefs(state: AppState): (string | undefined)[] {
	const [nav] = findAll(render(state, intents), byTag("nav"));
	return nav ? findAll(nav, byTag("a")).map((a) => a.attrs.href) : [];
}

describe("the layout", () => {
	test("an unknown session shows neither the login nor the app", () => {
		const view = render({ session: { status: "unknown" }, route: { name: "spaces" }, view: { status: "idle" } }, intents);
		expect(findAll(view, byTag("form"))).toEqual([]);
		expect(findAll(view, byTag("nav"))).toEqual([]);
	});

	test("a signed-out session shows the login", () => {
		const view = render({ session: { status: "signed-out" }, route: { name: "spaces" }, view: { status: "idle" } }, intents);
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

	test("the space list shows its data once it is loaded", () => {
		const state: AppState = { ...signedIn(admin), view: { status: "ready", data: listData, at: 1 } };
		expect(findAll(render(state, intents), byTag("table"))).toHaveLength(1);
	});

	test("a space page shows its detail once it is loaded", () => {
		const route: Route = { name: "space", space: "demo" };
		const state: AppState = { ...signedIn(admin, route), view: { status: "ready", data: { kind: "space", detail: spaceDetail }, at: 1 } };
		expect(findAll(render(state, intents), byTag("h1")).map(textOf)).toEqual(["demo"]);
	});

	const pageCases: [Route, ViewData, string][] = [
		[{ name: "board", space: "demo", board: "b" }, { kind: "board", space: "demo", board: "b", threads: threadInfos }, "b"],
		[{ name: "thread", space: "demo", threadId: 1 }, { kind: "thread", space: "demo", thread }, "plan"],
		[{ name: "inbox" }, { kind: "inbox", entries }, "Inbox of admin admin"],
		[{ name: "admins" }, { kind: "admins", admins }, "Admins"],
	];
	for (const [route, data, title] of pageCases) {
		test(`the ${route.name} page shows its data once it is loaded`, () => {
			const state: AppState = { ...signedIn(admin, route), view: { status: "ready", data, at: 1 } };
			expect(findAll(render(state, intents), byTag("h1")).map(textOf)).toEqual([title]);
		});
	}

	test("a secret shown once and a notice appear above the page", () => {
		const secret: AppState = { ...signedIn(admin), shownOnce: { label: "Token of admin ops", secret: "amb_admin_x" } };
		expect(findAll(render(secret, intents), byTag("code")).map(textOf)).toEqual(["amb_admin_x"]);
		const notice: AppState = { ...signedIn(admin), notice: { message: "agent lead is a leader agent", alert: true } };
		const [alert] = findAll(render(notice, intents), (n) => n.attrs.role === "alert");
		expect(alert && textOf(alert)).toBe("agent lead is a leader agent");
	});

	test("a success notice is a status, not an alert", () => {
		const notice: AppState = { ...signedIn(admin), notice: { message: "Admin ops revoked.", alert: false } };
		const [status] = findAll(render(notice, intents), (n) => n.attrs.role === "status");
		expect(status && textOf(status)).toBe("Admin ops revoked.");
	});

	test("the first load of a page says it is loading", () => {
		expect(mainOf({ ...signedIn(admin), view: { status: "loading" } })).toContain("Loading");
	});

	test("a failed load shows the server's message and keeps the last data", () => {
		const error = new ApiError(0, "network", "the server is unreachable");
		const state: AppState = { ...signedIn(admin), view: { status: "error", error, last: listData } };
		const view = render(state, intents);
		const [alert] = findAll(view, (n) => n.attrs.role === "alert");
		expect(alert && textOf(alert)).toBe("the server is unreachable");
		expect(findAll(view, byTag("table"))).toHaveLength(1);
	});

	test("a forbidden or unknown space shows only the server's message", () => {
		const error = new ApiError(404, "not_found", "space ghost not found");
		const state: AppState = { ...signedIn(admin, { name: "space", space: "ghost" }), view: { status: "error", error } };
		expect(mainOf(state)).toBe("space ghost not found");
	});

	test("data of another page is never shown", () => {
		const state: AppState = { ...signedIn(admin, { name: "space", space: "demo" }), view: { status: "ready", data: listData, at: 1 } };
		expect(findAll(render(state, intents), byTag("table"))).toEqual([]);
	});
});
