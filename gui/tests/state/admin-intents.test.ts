import { describe, expect, test } from "bun:test";
import type { AdminsApi } from "../../src/api/admins.ts";
import type { Action } from "../../src/state/actions.ts";
import { createAdminIntents } from "../../src/state/admin-intents.ts";

function intents(api: AdminsApi) {
	const actions: Action[] = [];
	return { actions, admin: createAdminIntents(api, { dispatch: (a) => actions.push(a), refresh: () => {} }) };
}

const api: AdminsApi = { mint: async (name) => ({ name, token: "amb_admin_x" }), revoke: async (name) => name === "ops" };

describe("the admin intents", () => {
	test("a minted token is shown once with the admin it belongs to", async () => {
		const { actions, admin } = intents(api);
		await admin.mint("ops");
		expect(actions).toEqual([{ type: "secret/shown", label: "Token of admin ops", secret: "amb_admin_x" }]);
	});

	test("a revoke says what it removed", async () => {
		const { actions, admin } = intents(api);
		await admin.revoke("ops");
		await admin.revoke("ghost");
		expect(actions).toEqual([
			{ type: "notice/shown", message: "Admin ops revoked.", alert: false },
			{ type: "notice/shown", message: "Nothing was removed: admin ghost has no token and joined no space.", alert: false },
		]);
	});
});
