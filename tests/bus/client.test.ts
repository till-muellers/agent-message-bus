import { describe, expect, test } from "bun:test";
import { BusError, createClient, type FetchInit, type FetchLike } from "../../extension/lib/bus/client.ts";
import { CODE, errorResponse, fakeBus, URL_ } from "../fakes.ts";

describe("client", () => {
	test("callTool posts JSON with bearer auth and unwraps result; errors become BusError", async () => {
		const bus = fakeBus({
			tools: {
				bus_whoami: () => Response.json({ result: { kind: "admin", spaces: [] } }),
				bus_fail: () => errorResponse(409, "conflict", "exists"),
			},
		});
		const client = createClient(bus.fetchFn);
		expect(await client.callTool(URL_, "t", "bus_whoami", {})).toEqual({ kind: "admin", spaces: [] });
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_whoami`, method: "POST", auth: "Bearer t" });
		const error = await client.callTool(URL_, "t", "bus_fail", {}).catch(e => e);
		expect(error).toBeInstanceOf(BusError);
		expect(error).toMatchObject({ code: "conflict", message: "exists" });
	});

	test("callTool with a transport timeout aborts the request when the time runs out, keeps Bun's idle timer as long, and passes the caller's signal on", async () => {
		const inits: FetchInit[] = [];
		const hanging: FetchLike = (_url, init) => {
			inits.push(init);
			const { promise, reject } = Promise.withResolvers<Response>();
			init.signal?.addEventListener("abort", () => reject(init.signal?.reason));
			return promise;
		};
		const client = createClient(hanging);
		const error = await client.callTool(URL_, "t", "bus_wait", {}, { timeoutMs: 5 }).catch(e => e);
		expect(error).toMatchObject({ name: "TimeoutError" });
		const controller = new AbortController();
		const cancelled = client.callTool(URL_, "t", "bus_wait", {}, { timeoutMs: 60_000, signal: controller.signal }).catch(e => e);
		controller.abort();
		expect(await cancelled).toMatchObject({ name: "AbortError" });
		expect(inits.map(init => [init.signal !== undefined, init.timeout])).toEqual([
			[true, 5],
			[true, 60_000],
		]);
	});

	test("acceptInvite posts the code as bearer with no body and returns name, space and token", async () => {
		const bus = fakeBus();
		const client = createClient(bus.fetchFn);
		expect(await client.acceptInvite(URL_, "K7M2-X9PQ")).toEqual({ name: "worker", space: "demo", token: "acc-tok" });
		expect(bus.calls).toEqual([{ url: `${URL_}/v1/invites/accept`, method: "POST", auth: "Bearer K7M2-X9PQ", body: undefined }]);
		await expect(client.acceptInvite(URL_, "nope-nope")).rejects.toMatchObject({ code: "unauthenticated", message: "invalid or expired invite code" });
	});

	test("acceptInvite returns the member, its space and its token", async () => {
		const client = createClient(fakeBus().fetchFn);
		expect(await client.acceptInvite(URL_, CODE)).toEqual({ name: "worker", space: "demo", token: "acc-tok" });
	});

	test("acceptInvite rejects a response without name, space and token", async () => {
		for (const [body, message] of [
			[{ name: "worker", space: "demo" }, "accept: response has no token"],
		] as const) {
			const error = await createClient(async () => Response.json(body))
				.acceptInvite(URL_, CODE)
				.catch(e => e);
			expect(error).toBeInstanceOf(BusError);
			expect(error).toMatchObject({ code: "invalid_response", message });
		}
	});

	test("fetchGuide gets the guide with bearer auth as text; a server without it fails with its error", async () => {
		const bus = fakeBus({ guide: () => new Response("# Guide", { headers: { "Content-Type": "text/markdown; charset=utf-8" } }) });
		expect(await createClient(bus.fetchFn).fetchGuide(URL_, "t")).toBe("# Guide");
		expect(bus.calls).toEqual([{ url: `${URL_}/v1/guide`, method: "GET", auth: "Bearer t", body: undefined }]);
		const old = fakeBus({ guide: () => new Response("404 page not found", { status: 404 }) });
		await expect(createClient(old.fetchFn).fetchGuide(URL_, "t")).rejects.toMatchObject({ code: "http_404" });
	});
});
