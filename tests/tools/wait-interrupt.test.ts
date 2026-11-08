import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { RemoteTool } from "../../extension/lib/bus/client.ts";
import { createWaiters } from "../../extension/lib/delivery/waiters.ts";
import { waitToolDefinition } from "../../extension/lib/tools/wait-tool.ts";
import { entry, z } from "../fakes.ts";

const TOOL_INTERRUPT = Symbol.for("pi-agent-core.tool-interrupt");
const serverWait: RemoteTool = {
	name: "bus_wait",
	description: "Waits until mail arrives.",
	inputSchema: { type: "object", properties: { timeoutSeconds: { type: "integer" } }, required: [], additionalProperties: false },
};

/**
 * A wait tool over a real waiter registry. `handOver` plays omp: it queues an entry as an aside and, like omp's agent
 * loop, aborts the signal of the running tool with the interrupt reason only when that tool is `interruptible`.
 * `waiters` is the registry the wait registered in: it must hold no stale waiter once the wait ended.
 */
function setup() {
	const waiters = createWaiters();
	const tool = waitToolDefinition({ zod: z } as unknown as ExtensionAPI, serverWait, {
		source: () => ({ takeHeld: () => [], awaitEntry: (filter, ms, signal) => waiters.wait(filter, ms, signal) }),
		claims: { claimFromServer: entries => [...entries] },
		forward: async () => [],
	});
	const controller = new AbortController();
	const handOver = () => {
		if (tool.interruptible === true) controller.abort(TOOL_INTERRUPT);
	};
	const run = async (): Promise<unknown> => {
		const [first] = (await tool.execute("id", { timeoutSeconds: 60 }, controller.signal, undefined, undefined as never)).content;
		return first?.type === "text" ? first.text : undefined;
	};
	return { run, handOver, waiters };
}

/** Lets the promise chains of an abort settle without a timer. */
const flushMicrotasks = async (): Promise<void> => {
	for (let i = 0; i < 20; i++) await Promise.resolve();
};

describe("an entry handed to omp while bus_wait runs", () => {
	test("ends the wait with [] at once and leaves the entry to omp", async () => {
		const { run, handOver, waiters } = setup();
		let answer: unknown = "still waiting";
		const pending = run().then(result => (answer = result));
		handOver();
		await flushMicrotasks();
		expect(answer).toBe("[]");
		await pending;
		expect(waiters.take(entry(5))).toBe(false);
	});
});
