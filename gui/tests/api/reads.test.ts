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
});
