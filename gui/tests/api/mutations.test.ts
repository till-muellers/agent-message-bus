import { describe, expect, test } from "bun:test";
import { createHttp } from "../../src/api/http.ts";
import { createMutations, type Mutations } from "../../src/api/mutations.ts";
import { type FakeFetch, fakeFetch, json, type RecordedRequest } from "../fake-fetch.ts";

function api(...responses: Response[]): { fake: FakeFetch; mutations: Mutations } {
	const fake = fakeFetch(...responses);
	return { fake, mutations: createMutations(createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" })) };
}

function sent(fake: FakeFetch): Pick<RecordedRequest, "method" | "path" | "body"> & { csrf: string | undefined } {
	const [r] = fake.requests;
	return { method: r?.method ?? "", path: r?.path ?? "", body: r?.body, csrf: r?.headers["x-csrf-token"] };
}

describe("the mutations", () => {
	test("an invite posts the name and task and returns the code once", async () => {
		const { fake, mutations } = api(json(200, { name: "new", space: "demo", code: "33p8-76ch", expires_at: "2026-11-04T21:00:00Z" }));
		expect(await mutations.invite("demo", "new", "do it")).toEqual({ name: "new", space: "demo", code: "33p8-76ch", expiresAt: "2026-11-04T21:00:00Z" });
		expect(sent(fake)).toEqual({ method: "POST", path: "/web/api/spaces/demo/invites", body: JSON.stringify({ name: "new", task: "do it" }), csrf: "c1" });
	});

	test("an invite without a task sends no task", async () => {
		const { fake, mutations } = api(json(200, { name: "new", space: "demo", code: "c", expires_at: "x" }));
		await mutations.invite("demo", "new", "");
		expect(fake.requests[0]?.body).toBe(JSON.stringify({ name: "new" }));
	});

	test("cancelling an invite deletes it by id and reports whether it was pending", async () => {
		const { fake, mutations } = api(json(200, { cancelled: false }));
		expect(await mutations.cancelInvite("demo", "3f9a0c1d2e4b5a67")).toBe(false);
		expect(sent(fake)).toMatchObject({ method: "DELETE", path: "/web/api/spaces/demo/invites/3f9a0c1d2e4b5a67", csrf: "c1" });
	});

	test("a revoke sends the confirmation <space>/<name>", async () => {
		const { fake, mutations } = api(json(200, { deleted: true }));
		expect(await mutations.revoke("demo", "worker")).toBe(true);
		expect(sent(fake)).toEqual({ method: "POST", path: "/web/api/spaces/demo/revoke", body: JSON.stringify({ name: "worker", confirm: "demo/worker" }), csrf: "c1" });
	});

	test("a post adds the body to the thread", async () => {
		const { fake, mutations } = api(json(200, { postId: 5, receipts: [] }));
		expect(await mutations.post("demo", 1, "reply")).toBe(5);
		expect(sent(fake)).toEqual({ method: "POST", path: "/web/api/spaces/demo/threads/1/posts", body: JSON.stringify({ body: "reply" }), csrf: "c1" });
	});

	test("a topic message sends the body and the critical flag", async () => {
		const { fake, mutations } = api(json(200, { ok: true, receipts: [] }));
		await mutations.sendTopic("demo", "all", "now", true);
		expect(sent(fake)).toEqual({ method: "POST", path: "/web/api/spaces/demo/topics/all/messages", body: JSON.stringify({ body: "now", critical: true }), csrf: "c1" });
	});

	test("deleting a space sends the confirmation <space>", async () => {
		const { fake, mutations } = api(json(200, { ok: true }));
		await mutations.deleteSpace("demo");
		expect(sent(fake)).toEqual({ method: "DELETE", path: "/web/api/spaces/demo", body: JSON.stringify({ confirm: "demo" }), csrf: "c1" });
	});
});
