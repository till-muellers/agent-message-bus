import { describe, expect, test } from "bun:test";
import { createHttp } from "../../src/api/http.ts";
import { createReads } from "../../src/api/reads.ts";
import { fakeFetch, json } from "../fake-fetch.ts";

const you = { kind: "admin", name: "admin", spaces: [] };
const emptyOverview = { space: "a b", you: { name: "admin", role: null }, participants: [], topics: [], blackboards: [], recentThreads: [] };

describe("the state reads", () => {
	test("the space list reads GET /web/api/state", async () => {
		const fake = fakeFetch(json(200, { you, spaces: [], inbox: { waiting: 0 } }));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		expect(await reads.spaces()).toEqual({ spaces: [], inboxWaiting: 0 });
		expect(fake.requests[0]).toMatchObject({ method: "GET", path: "/web/api/state" });
	});

	test("a space reads GET /web/api/state?space= with the name encoded", async () => {
		const fake = fakeFetch(json(200, { you, overview: emptyOverview, inbox: { waiting: 0 } }));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		await reads.space("a b");
		expect(fake.requests[0]?.path).toBe("/web/api/state?space=a%20b");
	});

	test("the threads of a blackboard read their route with the names encoded", async () => {
		const fake = fakeFetch(json(200, []));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		expect(await reads.threads("a b", "c")).toEqual([]);
		expect(fake.requests[0]).toMatchObject({ method: "GET", path: "/web/api/spaces/a%20b/blackboards/c/threads" });
	});

	test("a thread reads its route by id", async () => {
		const fake = fakeFetch(json(200, { id: 7, blackboard: "b", title: "t", createdBy: "w", subscribed: false, posts: [] }));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		await reads.thread("demo", 7);
		expect(fake.requests[0]?.path).toBe("/web/api/spaces/demo/threads/7");
	});

	test("the inbox reads GET /web/api/inbox and never a stream or an ack", async () => {
		const fake = fakeFetch(json(200, []));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		expect(await reads.inbox()).toEqual([]);
		expect(fake.requests).toEqual([expect.objectContaining({ method: "GET", path: "/web/api/inbox?limit=500" })]);
	});

	test("the admin list reads GET /web/api/admins", async () => {
		const fake = fakeFetch(json(200, []));
		const reads = createReads(createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined }));
		expect(await reads.admins()).toEqual([]);
		expect(fake.requests[0]).toMatchObject({ method: "GET", path: "/web/api/admins" });
	});
});
