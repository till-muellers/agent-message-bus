import { describe, expect, test } from "bun:test";
import type { Mutations } from "../../src/api/mutations.ts";
import type { Action } from "../../src/state/actions.ts";
import type { Route } from "../../src/state/app-state.ts";
import { createSpaceIntents, type SpaceIntents } from "../../src/state/space-intents.ts";

const invite = { id: "3f9a0c1d2e4b5a67", name: "new", invitedBy: "lead", expiresAt: "x" };

function intents(over: Partial<Mutations> = {}, here: Route = { name: "space", space: "demo" }): { actions: Action[]; calls: string[]; space: SpaceIntents } {
	const actions: Action[] = [];
	const calls: string[] = [];
	const api: Mutations = {
		invite: async (space, name) => ({ name, space, code: "33p8-76ch", expiresAt: "x" }),
		cancelInvite: async () => true,
		revoke: async (space, name) => calls.push(`revoke ${space}/${name}`) > 0,
		post: async () => 5,
		sendTopic: async () => true,
		deleteSpace: async (space) => {
			calls.push(`delete ${space}`);
			return true;
		},
		...over,
	};
	const deps = { dispatch: (a: Action) => void actions.push(a), refresh: () => {} };
	const nav = { current: () => here, go: (route: Route): void => void actions.push({ type: "navigate", route }) };
	return { actions, calls, space: createSpaceIntents(api, deps, nav) };
}

describe("the space intents", () => {
	test("an invite code is shown once with the name and space", async () => {
		const { actions, space } = intents();
		await space.invite("demo", "new", "");
		expect(actions).toEqual([{ type: "secret/shown", label: "Invite code for new in space demo", secret: "33p8-76ch" }]);
	});

	test("a cancelled invite says so; one no longer pending says why", async () => {
		const done = intents();
		await done.space.cancelInvite("demo", invite);
		const gone = intents({ cancelInvite: async () => false });
		await gone.space.cancelInvite("demo", invite);
		expect([...done.actions, ...gone.actions]).toEqual([
			{ type: "notice/shown", message: "Invite for new cancelled.", alert: false },
			{ type: "notice/shown", message: "The invite was already used, expired or cancelled.", alert: false },
		]);
	});

	test("asking opens the confirmation and dismissing closes it", () => {
		const { actions, space } = intents();
		space.ask({ kind: "delete-space", space: "demo" });
		space.dismiss();
		expect(actions).toEqual([{ type: "confirm/open", confirm: { kind: "delete-space", space: "demo" } }, { type: "confirm/close" }]);
	});

	test("a confirmed revoke closes the confirmation, revokes and says what it removed", async () => {
		const { actions, calls, space } = intents();
		await space.confirmed({ kind: "revoke", space: "demo", name: "worker", role: "member" });
		expect(calls).toEqual(["revoke demo/worker"]);
		expect(actions).toEqual([{ type: "confirm/close" }, { type: "notice/shown", message: "Member worker revoked from space demo.", alert: false }]);
	});

	test("a confirmed delete deletes the space, goes to the space list and says so there", async () => {
		const { actions, calls, space } = intents();
		await space.confirmed({ kind: "delete-space", space: "demo" });
		expect(calls).toEqual(["delete demo"]);
		expect(actions).toEqual([
			{ type: "confirm/close" },
			{ type: "navigate", route: { name: "spaces" } },
			{ type: "notice/shown", message: "Space demo deleted.", alert: false },
		]);
	});

	const elsewhere: [string, Route, boolean][] = [
		["a board of the deleted space", { name: "board", space: "demo", board: "b" }, true],
		["a thread of the deleted space", { name: "thread", space: "demo", threadId: 1 }, true],
		["another space", { name: "space", space: "other" }, false],
		["the inbox", { name: "inbox" }, false],
	];
	for (const [where, here, leaves] of elsewhere) {
		test(`a delete answered while the operator is on ${where} ${leaves ? "goes to the space list" : "keeps them there"}`, async () => {
			const { actions, space } = intents({}, here);
			await space.confirmed({ kind: "delete-space", space: "demo" });
			const navigated = actions.some((a) => a.type === "navigate");
			expect(navigated).toBe(leaves);
			expect(actions.at(-1)).toEqual({ type: "notice/shown", message: "Space demo deleted.", alert: false });
		});
	}

	test("a refused delete stays on the space", async () => {
		const { actions, space } = intents({ deleteSpace: async () => Promise.reject(new Error("no")) });
		await space.confirmed({ kind: "delete-space", space: "demo" });
		expect(actions).toEqual([{ type: "confirm/close" }, { type: "notice/shown", message: "no", alert: true }]);
	});

	test("a post and a topic message report their success", async () => {
		const { actions, space } = intents();
		await space.post("demo", 1, "reply");
		await space.send("demo", "all", "hi", false);
		expect(actions).toEqual([
			{ type: "notice/shown", message: "Post 5 added.", alert: false },
			{ type: "notice/shown", message: "Sent to topic all.", alert: false },
		]);
	});
});
