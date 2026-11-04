import { describe, expect, test } from "bun:test";
import { ApiError } from "../../src/api/api-error.ts";
import { createHttp } from "../../src/api/http.ts";
import { fakeFetch, json } from "../fake-fetch.ts";

const passThrough = (value: unknown): unknown => value;

describe("the HTTP client", () => {
	test("GET sends cookies of the same origin, asks for JSON and carries no CSRF token", async () => {
		const fake = fakeFetch(json(200, { a: 1 }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" });
		expect(await http.get("/web/api/state", passThrough)).toEqual({ a: 1 });
		const [req] = fake.requests;
		expect(req?.method).toBe("GET");
		expect(req?.credentials).toBe("same-origin");
		expect(req?.headers.accept).toBe("application/json");
		expect(req?.headers["x-csrf-token"]).toBeUndefined();
	});

	test("a mutation carries the session's CSRF token and a JSON body", async () => {
		const fake = fakeFetch(json(200, { ok: true }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" });
		await http.send("POST", "/web/api/admins", { name: "ops" }, passThrough);
		const [req] = fake.requests;
		expect(req?.method).toBe("POST");
		expect(req?.headers["x-csrf-token"]).toBe("c1");
		expect(req?.headers["content-type"]).toBe("application/json");
		expect(req?.body).toBe('{"name":"ops"}');
	});

	test("a CSRF-exempt request carries the given headers and no CSRF token", async () => {
		const fake = fakeFetch(json(200, { ok: true }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => "c1" });
		await http.exempt("POST", "/web/session", { Authorization: "Bearer t" }, passThrough);
		const [req] = fake.requests;
		expect(req?.headers.authorization).toBe("Bearer t");
		expect(req?.headers["x-csrf-token"]).toBeUndefined();
		expect(req?.body).toBeUndefined();
	});

	test("the error envelope becomes an ApiError with the server's code and message", async () => {
		const fake = fakeFetch(json(429, { error: { code: "rate_limited", message: "too many failed sign-ins; try again later" } }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined });
		const failure = http.get("/web/session", passThrough);
		await expect(failure).rejects.toBeInstanceOf(ApiError);
		await expect(failure).rejects.toMatchObject({
			status: 429,
			code: "rate_limited",
			message: "too many failed sign-ins; try again later",
		});
	});

	test("an error without an envelope is named by its status", async () => {
		const fake = fakeFetch(new Response("boom", { status: 502 }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined });
		await expect(http.get("/web/api/state", passThrough)).rejects.toMatchObject({ status: 502, code: "http_error" });
	});

	test("an unreachable server is a network error", async () => {
		const fake = fakeFetch(new TypeError("Failed to fetch"));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined });
		await expect(http.get("/web/api/state", passThrough)).rejects.toMatchObject({ status: 0, code: "network" });
	});

	test("a success that is not JSON is an invalid response", async () => {
		const fake = fakeFetch(new Response("<html>", { status: 200 }));
		const http = createHttp({ fetchFn: fake.fetchFn, csrf: () => undefined });
		await expect(http.get("/web/api/state", passThrough)).rejects.toMatchObject({ code: "invalid_response" });
	});
});
