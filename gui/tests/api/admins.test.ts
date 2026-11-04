import { describe, expect, test } from "bun:test";
import { type AdminsApi, createAdminsApi } from "../../src/api/admins.ts";
import { createHttp } from "../../src/api/http.ts";
import { type FakeFetch, fakeFetch, json } from "../fake-fetch.ts";

function api(...responses: Response[]): { fake: FakeFetch; admins: AdminsApi } {
	const fake = fakeFetch(...responses);
	return { fake, admins: createAdminsApi(createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" })) };
}

describe("the admin mutations", () => {
	test("minting posts the name with the CSRF token", async () => {
		const { fake, admins } = api(json(200, { name: "ops", token: "amb_admin_x" }));
		expect(await admins.mint("ops")).toEqual({ name: "ops", token: "amb_admin_x" });
		expect(fake.requests[0]).toMatchObject({ method: "POST", path: "/web/api/admins", body: JSON.stringify({ name: "ops" }) });
		expect(fake.requests[0]?.headers["x-csrf-token"]).toBe("c1");
	});

	test("revoking posts to the admin's revoke route with the CSRF token", async () => {
		const { fake, admins } = api(json(200, { deleted: true }));
		expect(await admins.revoke("a b")).toBe(true);
		expect(fake.requests[0]).toMatchObject({ method: "POST", path: "/web/api/admins/a%20b/revoke" });
		expect(fake.requests[0]?.headers["x-csrf-token"]).toBe("c1");
	});

	test("a refused mint carries the server's message", async () => {
		const { admins } = api(json(409, { error: { code: "conflict", message: "agent lead is a leader agent" } }));
		await expect(admins.mint("lead")).rejects.toMatchObject({ status: 409, message: "agent lead is a leader agent" });
	});
});
