import { afterEach, describe, expect, setSystemTime, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { RemoteTool } from "../../extension/lib/bus/client.ts";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import type { WaitFilter } from "../../extension/lib/delivery/waiters.ts";
import { type WaitToolDeps, waitToolDefinition } from "../../extension/lib/tools/wait-tool.ts";
import { entry, type FakeNode, toJson, z } from "../fakes.ts";

const serverWait: RemoteTool = {
	name: "bus_wait",
	description: "Waits until mail arrives.",
	inputSchema: {
		type: "object",
		properties: {
			timeoutSeconds: { type: "integer", description: "Seconds to wait, 1..300 (default 30)." },
			after: { type: "integer" },
			from: { type: "string" },
			threadId: { type: "integer" },
		},
		required: [],
		additionalProperties: false,
	},
};

const pi = { zod: z } as unknown as ExtensionAPI;

/**
 * A wait tool over a live stream whose ledger holds `held` and answers `entries`, recording waits, forwards and
 * claims; the forwarded call answers `answer`, or what `answer` returns per call when it is a function, and the
 * stream already delivered the ids in `delivered`.
 */
function setup(live: boolean, entries: Entry[] = [], held: Entry[] = [], answer: unknown = [entry(9)], delivered: number[] = []) {
	const waits: { filter: WaitFilter; ms: number; signal?: AbortSignal }[] = [];
	const forwards: { args: unknown; ms: number }[] = [];
	const claims: number[][] = [];
	const takes: WaitFilter[] = [];
	const stream = { live };
	const deps: WaitToolDeps = {
		source: () =>
			stream.live
				? {
						takeHeld: filter => {
							takes.push(filter);
							return held.splice(0);
						},
						awaitEntry: async (filter, ms, signal) => {
							waits.push({ filter, ms, signal });
							return entries;
						},
					}
				: undefined,
		claims: {
			claimFromServer: answered => {
				claims.push(answered.map(e => e.id));
				return answered.filter(e => !delivered.includes(e.id));
			},
		},
		forward: async (args, ms) => {
			forwards.push({ args, ms });
			return typeof answer === "function" ? (answer as () => unknown)() : answer;
		},
	};
	const tool = waitToolDefinition(pi, serverWait, deps);
	const run = async (params: Record<string, unknown>, signal?: AbortSignal) => {
		const [first] = (await tool.execute("id", params, signal, undefined, undefined as never)).content;
		if (first?.type !== "text") throw new Error("bus_wait returned no text");
		return JSON.parse(first.text) as unknown;
	};
	return { tool, run, waits, forwards, claims, takes, stream };
}

describe("the local bus_wait", () => {
	test("it has the server's name, description and schema", () => {
		const { tool } = setup(true);
		expect(tool.name).toBe("bus_wait");
		expect(tool.description).toBe(serverWait.description);
		expect(toJson(tool.parameters as unknown as FakeNode)).toEqual(serverWait.inputSchema);
	});

	test("it is a top-level tool that needs read approval", () => {
		const { tool } = setup(true);
		expect([tool.loadMode, tool.approval]).toEqual(["essential", "read"]);
	});

	test("with a live stream it waits in the ledger and claims nothing", async () => {
		const { run, waits, forwards, claims } = setup(true, [entry(4, { from: "lead" })]);
		expect(await run({ from: "lead", threadId: 7, after: 3, timeoutSeconds: 5 })).toEqual([entry(4, { from: "lead" })]);
		expect(waits.map(w => [w.filter, w.ms])).toEqual([[{ from: "lead", threadId: 7, after: 3 }, 5000]]);
		expect(forwards).toEqual([]);
		expect(claims).toEqual([]);
	});

	test("entries the ledger still holds are returned at once, without waiting", async () => {
		const held = [entry(1, { type: "thread_post", threadId: 7 }), entry(2, { type: "thread_post", threadId: 7 })];
		const { run, waits, takes } = setup(true, [], held);
		expect(await run({ threadId: 7, timeoutSeconds: 5 })).toEqual([entry(1, { type: "thread_post", threadId: 7 }), entry(2, { type: "thread_post", threadId: 7 })]);
		expect(takes).toEqual([{ from: undefined, threadId: 7, after: undefined }]);
		expect(waits).toEqual([]);
	});

	test("it waits 30 seconds by default and returns [] when nothing came", async () => {
		const { run, waits } = setup(true);
		expect(await run({})).toEqual([]);
		expect(waits.map(w => w.ms)).toEqual([30_000]);
	});

	test("the turn's signal cancels the wait", async () => {
		const { run, waits } = setup(true);
		const controller = new AbortController();
		await run({}, controller.signal);
		expect(waits[0].signal).toBe(controller.signal);
	});

	test("without a live stream it forwards to the server's bus_wait with timeoutSeconds + 10 s; what it returns is claimed", async () => {
		const { run, forwards, claims } = setup(false);
		expect(await run({ from: "lead", timeoutSeconds: 10 })).toEqual([entry(9)]);
		await run({});
		expect(forwards).toEqual([
			{ args: { from: "lead", timeoutSeconds: 10 }, ms: 20_000 },
			{ args: {}, ms: 40_000 },
		]);
		expect(claims).toEqual([[9], [9]]);
	});

	test("a forwarded entry the session's stream already delivered is left out; a result that is no entry list passes through", async () => {
		const delivered = setup(false, [], [], [entry(8), entry(9)], [8]);
		expect(await delivered.run({})).toEqual([entry(9)]);
		expect(delivered.claims).toEqual([[8, 9]]);
		const odd = setup(false, [], [], { unexpected: true });
		expect(await odd.run({})).toEqual({ unexpected: true });
		expect(odd.claims).toEqual([]);
	});

	describe("when every forwarded entry was delivered by the session's stream", () => {
		afterEach(() => setSystemTime());

		/** Answers the forwarded calls in order, advancing the clock by `elapsedMs` per call. */
		function answers(elapsedMs: number, ...results: Entry[][]) {
			setSystemTime(new Date("2026-11-05T00:00:00Z"));
			return () => {
				setSystemTime(new Date(Date.now() + elapsedMs));
				return results.shift();
			};
		}

		test("it forwards again after the newest of them for the time left and returns the next new entry", async () => {
			const { run, forwards, claims } = setup(false, [], [], answers(4000, [entry(5), entry(6)], [entry(7)]), [5, 6]);
			expect(await run({ from: "lead", after: 2, timeoutSeconds: 10 })).toEqual([entry(7)]);
			expect(forwards.map(f => f.args)).toEqual([
				{ from: "lead", after: 2, timeoutSeconds: 10 },
				{ from: "lead", after: 6, timeoutSeconds: 6 },
			]);
			expect(claims).toEqual([[5, 6], [7]]);
		});

		test("it returns [] once the server answers [] or the deadline passed", async () => {
			const empty = setup(false, [], [], answers(1000, [entry(5)], []), [5]);
			expect(await empty.run({ timeoutSeconds: 10 })).toEqual([]);
			expect(empty.forwards.map(f => f.args)).toEqual([{ timeoutSeconds: 10 }, { after: 5, timeoutSeconds: 9 }]);
			const late = setup(false, [], [], answers(10_000, [entry(5)], [entry(6)]), [5]);
			expect(await late.run({ timeoutSeconds: 10 })).toEqual([]);
			expect(late.forwards.map(f => f.args)).toEqual([{ timeoutSeconds: 10 }]);
		});

		test("once the session's own stream is open it waits there after the newest of them for the time left", async () => {
			const next = answers(3000, [entry(5)]);
			const streams: { live: boolean }[] = [];
			const open = () => {
				for (const stream of streams) stream.live = true;
				return next();
			};
			const it = setup(false, [entry(7)], [], open, [5]);
			streams.push(it.stream);
			expect(await it.run({ from: "lead", timeoutSeconds: 10 })).toEqual([entry(7)]);
			expect(it.forwards.map(f => f.args)).toEqual([{ from: "lead", timeoutSeconds: 10 }]);
			expect(it.takes).toEqual([{ from: "lead", threadId: undefined, after: 5 }]);
			expect(it.waits.map(w => [w.filter, w.ms])).toEqual([[{ from: "lead", threadId: undefined, after: 5 }, 7000]]);
			expect(it.claims).toEqual([[5]]);
		});
	});

	test("a timeout outside 1..300 is refused as the server refuses it", async () => {
		const { run, waits } = setup(true);
		for (const timeoutSeconds of [0, 301]) await expect(run({ timeoutSeconds })).rejects.toThrow("invalid: timeoutSeconds must be 1..300");
		expect(waits).toEqual([]);
	});
});
