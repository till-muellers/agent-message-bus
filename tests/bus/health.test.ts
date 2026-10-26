import { describe, expect, test } from "bun:test";
import { probeProtocol } from "../../extension/lib/bus/health.ts";
import type { FetchLike } from "../../extension/lib/bus/client.ts";

/** A fetch that answers the health probe with the given headers. */
function fakeHealth(headers: Record<string, string>, status = 200): { fetchFn: FetchLike; urls: string[] } {
	const urls: string[] = [];
	const fetchFn: FetchLike = async url => {
		urls.push(url);
		return new Response("ok", { status, headers });
	};
	return { fetchFn, urls };
}

describe("probeProtocol", () => {
	test("reads the protocol version from the health header", async () => {
		const { fetchFn, urls } = fakeHealth({ "Agent-Bus-Protocol": "2" });
		expect(await probeProtocol(fetchFn, "http://bus.test")).toBe(2);
		expect(urls).toEqual(["http://bus.test/healthz"]);
	});

	test("strips the trailing slashes of the url", async () => {
		const { fetchFn, urls } = fakeHealth({ "Agent-Bus-Protocol": "3" });
		expect(await probeProtocol(fetchFn, "http://bus.test//")).toBe(3);
		expect(urls).toEqual(["http://bus.test/healthz"]);
	});

	test("is undefined when the server reports no protocol", async () => {
		const { fetchFn } = fakeHealth({});
		expect(await probeProtocol(fetchFn, "http://bus.test")).toBeUndefined();
	});

	test("rejects when the server does not answer the probe", async () => {
		const { fetchFn } = fakeHealth({}, 500);
		await expect(probeProtocol(fetchFn, "http://bus.test")).rejects.toThrow("500");
	});

	test("rejects when the request fails", async () => {
		const fetchFn: FetchLike = async () => {
			throw new Error("network down");
		};
		await expect(probeProtocol(fetchFn, "http://bus.test")).rejects.toThrow("network down");
	});
});
