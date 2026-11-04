import { describe, expect, test } from "bun:test";
import { ApiError } from "../../src/api/api-error.ts";
import type { SessionApi } from "../../src/api/session.ts";
import type { SessionInfo } from "../../src/api/types.ts";
import type { Action } from "../../src/state/actions.ts";
import { createSessionIntents } from "../../src/state/session-intents.ts";

const info: SessionInfo = { identity: { kind: "admin", name: "ops" }, csrf: "c1" };

function fakeApi(outcome: SessionInfo | ApiError): SessionApi {
	const settle = async (): Promise<SessionInfo> => {
		if (outcome instanceof ApiError) throw outcome;
		return outcome;
	};
	return { signIn: settle, restore: settle, signOut: async () => void (await settle()) };
}

function run(api: SessionApi): { actions: Action[]; intents: ReturnType<typeof createSessionIntents> } {
	const actions: Action[] = [];
	return { actions, intents: createSessionIntents(api, (action) => actions.push(action)) };
}

describe("session intents", () => {
	test("a successful sign-in restores the session", async () => {
		const { actions, intents } = run(fakeApi(info));
		await intents.signIn("tok");
		expect(actions).toEqual([{ type: "session/restored", identity: info.identity, csrf: "c1" }]);
	});

	test("a refused sign-in shows the server's message verbatim", async () => {
		const { actions, intents } = run(fakeApi(new ApiError(401, "unauthenticated", "invalid token")));
		await intents.signIn("nope");
		expect(actions).toEqual([{ type: "session/signed-out", error: "invalid token" }]);
	});

	test("a restore without a session shows the login without an error", async () => {
		const { actions, intents } = run(fakeApi(new ApiError(401, "unauthenticated", "session required")));
		await intents.restore();
		expect(actions).toEqual([{ type: "session/signed-out" }]);
	});

	test("a restore that fails for another reason says why", async () => {
		const { actions, intents } = run(fakeApi(new ApiError(0, "network", "the server is unreachable")));
		await intents.restore();
		expect(actions).toEqual([{ type: "session/signed-out", error: "the server is unreachable" }]);
	});

	test("signing out ends the session", async () => {
		const { actions, intents } = run(fakeApi(info));
		await intents.signOut();
		expect(actions).toEqual([{ type: "session/signed-out" }]);
	});

	test("a sign-out the server did not confirm says so", async () => {
		const { actions, intents } = run(fakeApi(new ApiError(0, "network", "the server is unreachable")));
		await intents.signOut();
		expect(actions).toEqual([
			{ type: "session/signed-out", error: "sign-out not confirmed by the server: the server is unreachable" },
		]);
	});
});
