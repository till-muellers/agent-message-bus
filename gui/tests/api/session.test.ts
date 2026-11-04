import { describe, expect, test } from "bun:test";
import { createHttp } from "../../src/api/http.ts";
import { createSessionApi } from "../../src/api/session.ts";
import { fakeFetch, json } from "../fake-fetch.ts";

const session = { kind: "admin", name: "admin", csrf: "c1" };

describe("the session API", () => {
	test("signing in sends the token as a bearer header to POST /web/session", async () => {
		const fake = fakeFetch(json(200, session));
		const api = createSessionApi(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		expect(await api.signIn("tok")).toEqual({ identity: { kind: "admin", name: "admin" }, csrf: "c1" });
		expect(fake.requests[0]).toMatchObject({ method: "POST", path: "/web/session" });
		expect(fake.requests[0]?.headers.authorization).toBe("Bearer tok");
	});

	test("restoring reads GET /web/session with the cookie only", async () => {
		const fake = fakeFetch(json(200, session));
		const api = createSessionApi(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		await api.restore();
		expect(fake.requests[0]).toMatchObject({ method: "GET", path: "/web/session" });
		expect(fake.requests[0]?.headers.authorization).toBeUndefined();
	});

	test("signing out sends DELETE /web/session without a CSRF token", async () => {
		const fake = fakeFetch(json(200, { ok: true }));
		const api = createSessionApi(createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" }));
		await api.signOut();
		expect(fake.requests[0]).toMatchObject({ method: "DELETE", path: "/web/session" });
		expect(fake.requests[0]?.headers["x-csrf-token"]).toBeUndefined();
	});
});
