import { describe, expect, test } from "bun:test";
import type { FetchLike } from "../../extension/lib/bus/client.ts";
import { createServerGate } from "../../extension/lib/connection/server-gate.ts";

/** A fetch that answers the health probe with the given protocol header (none when undefined). */
function fakeHealth(protocol: number | undefined): { fetchFn: FetchLike; calls: number } {
	const state = { calls: 0 };
	const fetchFn: FetchLike = async () => {
		state.calls++;
		const headers = protocol === undefined ? {} : { "Agent-Bus-Protocol": String(protocol) };
		return new Response("ok", { status: 200, headers });
	};
	return { fetchFn, calls: 0 };
}

describe("the server gate", () => {
	test("refuses a server that reports no protocol", async () => {
		const { fetchFn } = fakeHealth(undefined);
		const gate = createServerGate(fetchFn);
		expect(await gate.check("http://bus.test")).toEqual({
			refused: "the server at http://bus.test is too old (protocol none, this plugin needs 2); upgrade the agent-message-bus server",
		});
	});

	test("refuses a server below the minimum protocol", async () => {
		const { fetchFn } = fakeHealth(1);
		const gate = createServerGate(fetchFn);
		expect(await gate.check("http://bus.test")).toEqual({
			refused: "the server at http://bus.test is too old (protocol 1, this plugin needs 2); upgrade the agent-message-bus server",
		});
	});

	test("allows a server at or above the minimum protocol", async () => {
		for (const protocol of [2, 3]) {
			const { fetchFn } = fakeHealth(protocol);
			const gate = createServerGate(fetchFn);
			expect(await gate.check("http://bus.test")).toBe("ok");
		}
	});

	test("a failed probe is not a refusal", async () => {
		const fetchFn: FetchLike = async () => {
			throw new Error("network down");
		};
		const gate = createServerGate(fetchFn);
		expect(await gate.check("http://bus.test")).toBe("ok");
	});

	test("probes a url once", async () => {
		const { fetchFn } = fakeHealth(2);
		let calls = 0;
		const counting: FetchLike = async (url, init) => {
			calls++;
			return fetchFn(url, init);
		};
		const gate = createServerGate(counting);
		expect(await gate.check("http://bus.test")).toBe("ok");
		expect(await gate.check("http://bus.test/")).toBe("ok");
		expect(calls).toBe(1);
	});

	test("remembers the last answered probe for the status line", async () => {
		const { fetchFn } = fakeHealth(2);
		const gate = createServerGate(fetchFn);
		expect(gate.last()).toBeUndefined();
		await gate.check("http://bus.test");
		expect(gate.last()).toEqual({ url: "http://bus.test", protocol: 2 });
	});

	test("a server without a protocol is remembered as none", async () => {
		const { fetchFn } = fakeHealth(undefined);
		const gate = createServerGate(fetchFn);
		await gate.check("http://bus.test");
		expect(gate.last()).toEqual({ url: "http://bus.test", protocol: undefined });
	});
});
