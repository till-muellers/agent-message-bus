import { describe, expect, test } from "bun:test";
import { createAckWatermark } from "../../extension/lib/delivery/ack-watermark.ts";
import { createDispatcher } from "../../extension/lib/delivery/dispatcher.ts";
import { entry } from "../fakes.ts";

/** A dispatcher of an idle interactive session that records the ids it hands over to omp. */
function dispatcher() {
	const handed: number[] = [];
	const it = createDispatcher({
		phase: () => "interactive-idle",
		watermark: createAckWatermark(),
		handOver: message => void handed.push(...message.details.entryIds),
	});
	return { it, handed };
}

describe("the dispatcher's waiters (bus_wait)", () => {
	test("a waiter gets the next entry and it is not handed to omp", async () => {
		const { it, handed } = dispatcher();
		const wait = it.waitNext({}, 1000);
		it.receive(entry(1));
		expect((await wait).map(e => e.id)).toEqual([1]);
		expect(handed).toEqual([]);
	});

	test("an entry the waiter does not want is handed over the normal way", async () => {
		const { it, handed } = dispatcher();
		const fromLead = it.waitNext({ from: "lead" }, 1000);
		it.receive(entry(1, { from: "peer" }));
		it.receive(entry(2, { from: "lead" }));
		expect((await fromLead).map(e => e.id)).toEqual([2]);
		expect(handed).toEqual([1]);
	});

	test("a waiter can wait for one thread or for ids after a given one", async () => {
		const { it, handed } = dispatcher();
		const thread = it.waitNext({ threadId: 7 }, 1000);
		const after = it.waitNext({ after: 3 }, 1000);
		it.receive(entry(2, { type: "thread_post", threadId: 6 }));
		it.receive(entry(3, { type: "thread_post", threadId: 7 }));
		it.receive(entry(4));
		expect((await thread).map(e => e.id)).toEqual([3]);
		expect((await after).map(e => e.id)).toEqual([4]);
		expect(handed).toEqual([2]);
	});

	test("a wait that runs out returns [] and the entry after it goes to omp", async () => {
		const { it, handed } = dispatcher();
		expect(await it.waitNext({}, 1)).toEqual([]);
		it.receive(entry(1));
		expect(handed).toEqual([1]);
	});

	test("an aborted wait returns [] and takes nothing", async () => {
		const { it, handed } = dispatcher();
		const controller = new AbortController();
		const wait = it.waitNext({}, 60_000, controller.signal);
		controller.abort();
		expect(await wait).toEqual([]);
		it.receive(entry(1));
		expect(handed).toEqual([1]);
		const aborted = new AbortController();
		aborted.abort();
		expect(await it.waitNext({}, 60_000, aborted.signal)).toEqual([]);
	});

	test("an entry a waiter took counts as handed over, so a replay does not reach omp", async () => {
		const { it, handed } = dispatcher();
		const wait = it.waitNext({}, 1000);
		it.receive(entry(1));
		await wait;
		it.receive(entry(1));
		expect(handed).toEqual([]);
		expect(await it.handedUpTo(1, 0)).toBe(true);
	});
});
