import { describe, expect, test } from "bun:test";
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

/** A wait tool over a live stream whose dispatcher answers `entries`, recording waits, forwards and consumption. */
function setup(live: boolean, entries: Entry[] = []) {
	const waits: { filter: WaitFilter; ms: number; signal?: AbortSignal }[] = [];
	const forwards: { args: unknown; ms: number }[] = [];
	const consumed: number[][] = [];
	const deps: WaitToolDeps = {
		dispatcher: () =>
			live
				? {
						waitNext: async (filter, ms, signal) => {
							waits.push({ filter, ms, signal });
							return entries;
						},
					}
				: undefined,
		forward: async (args, ms) => {
			forwards.push({ args, ms });
			return [entry(9)];
		},
		consumed: ids => void consumed.push([...ids]),
	};
	const tool = waitToolDefinition(pi, serverWait, deps);
	const run = async (params: Record<string, unknown>, signal?: AbortSignal) => {
		const [first] = (await tool.execute("id", params, signal, undefined, undefined as never)).content;
		if (first?.type !== "text") throw new Error("bus_wait returned no text");
		return JSON.parse(first.text) as unknown;
	};
	return { tool, run, waits, forwards, consumed };
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

	test("with a live stream it waits in the dispatcher and the entries it returns are consumed", async () => {
		const { run, waits, forwards, consumed } = setup(true, [entry(4, { from: "lead" })]);
		expect(await run({ from: "lead", threadId: 7, after: 3, timeoutSeconds: 5 })).toEqual([entry(4, { from: "lead" })]);
		expect(waits.map(w => [w.filter, w.ms])).toEqual([[{ from: "lead", threadId: 7, after: 3 }, 5000]]);
		expect(forwards).toEqual([]);
		expect(consumed).toEqual([[4]]);
	});

	test("it waits 30 seconds by default and returns [] when nothing came", async () => {
		const { run, waits, consumed } = setup(true);
		expect(await run({})).toEqual([]);
		expect(waits.map(w => w.ms)).toEqual([30_000]);
		expect(consumed).toEqual([]);
	});

	test("the turn's signal cancels the wait", async () => {
		const { run, waits } = setup(true);
		const controller = new AbortController();
		await run({}, controller.signal);
		expect(waits[0].signal).toBe(controller.signal);
	});

	test("without a live stream it forwards to the server's bus_wait with timeoutSeconds + 10 s and consumes nothing", async () => {
		const { run, forwards, consumed } = setup(false);
		expect(await run({ from: "lead", timeoutSeconds: 10 })).toEqual([entry(9)]);
		await run({});
		expect(forwards).toEqual([
			{ args: { from: "lead", timeoutSeconds: 10 }, ms: 20_000 },
			{ args: {}, ms: 40_000 },
		]);
		expect(consumed).toEqual([]);
	});

	test("a timeout outside 1..300 is refused as the server refuses it", async () => {
		const { run, waits } = setup(true);
		for (const timeoutSeconds of [0, 301]) await expect(run({ timeoutSeconds })).rejects.toThrow("invalid: timeoutSeconds must be 1..300");
		expect(waits).toEqual([]);
	});
});
