import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { createAckWatermark } from "../../extension/lib/delivery/ack-watermark.ts";
import type { Mechanism, SessionPhase } from "../../extension/lib/delivery/delivery-class.ts";
import { createDispatcher } from "../../extension/lib/delivery/dispatcher.ts";
import type { BusMessage } from "../../extension/lib/render/details.ts";
import { entry } from "../fakes.ts";

/** A dispatcher of an idle interactive session that records the ids it hands over to omp and the ids it consumes. */
function dispatcher() {
	const handed: number[] = [];
	const consumed: number[][] = [];
	const it = createDispatcher({
		phase: () => "interactive-idle",
		watermark: createAckWatermark(),
		handOver: message => void handed.push(...message.details.entryIds),
		consume: ids => void consumed.push([...ids]),
		warn: () => {},
	});
	return { it, handed, consumed };
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

	test("an entry already handed to omp is never returned, not even when a reconnect replays it", async () => {
		const { it, handed } = dispatcher();
		it.receive(entry(1));
		const wait = it.waitNext({}, 1);
		it.receive(entry(1));
		expect(await wait).toEqual([]);
		expect(handed).toEqual([1]);
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

	test("claim returns the entries not handed over yet, and the stream skips them afterwards but counts them handed over", async () => {
		const { it, handed } = dispatcher();
		it.receive(entry(1));
		expect(it.claim([entry(1), entry(2)]).map(e => e.id)).toEqual([2]);
		it.receive(entry(2));
		expect(await it.handedUpTo(2, 0)).toBe(true);
		it.receive(entry(3));
		expect(handed).toEqual([1, 3]);
	});

	test("a claimed entry is consumed when the stream delivers it; a replayed entry omp got is not", () => {
		const { it, handed, consumed } = dispatcher();
		it.receive(entry(1));
		it.claim([entry(2)]);
		it.receive(entry(1));
		it.receive(entry(2));
		expect(handed).toEqual([1]);
		expect(consumed).toEqual([[2]]);
	});
});

/** A dispatcher whose phase can change, recording every message it hands over (with its mechanism) and every warning. */
function recording(initial: SessionPhase) {
	const watermark = createAckWatermark();
	const messages: { message: BusMessage; mechanism: Mechanism }[] = [];
	const warnings: string[] = [];
	const state = { phase: initial, fail: false };
	const it = createDispatcher({
		phase: () => state.phase,
		watermark,
		handOver: (message, mechanism) => {
			if (state.fail) throw new Error("omp refused the message");
			messages.push({ message, mechanism });
		},
		consume: () => {},
		warn: message => void warnings.push(message),
	});
	const handed = () => messages.map(m => [m.message.details.entryIds, m.mechanism.deliverAs]);
	return { it, messages, handed, watermark, state, warnings };
}

function post(id: number, threadId = 7, extra: Partial<Entry> = {}): Entry {
	return entry(id, { type: "thread_post", blackboard: "plan", threadId, threadTitle: "design", from: "peer", ...extra });
}

describe("blackboard notices in the dispatcher", () => {
	beforeEach(() => jest.useFakeTimers());
	afterEach(() => jest.useRealTimers());

	test("while busy, 3 posts to one thread within the window become one aside with 3 entryIds, acked together", () => {
		const { it, messages, handed, watermark } = recording("interactive-busy");
		for (const id of [1, 2, 3]) it.receive(post(id));
		expect(handed()).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([[[1, 2, 3], "aside"]]);
		expect(messages[0].message.content.match(/<agent-bus-entry /g)?.length).toBe(3);
		expect(watermark.consumed(messages[0].message.details.entryIds)).toBe(3);
	});

	test("posts to different threads stay separate notices", () => {
		const { it, handed } = recording("interactive-busy");
		it.receive(post(1, 7));
		it.receive(post(2, 8));
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([
			[[1], "aside"],
			[[2], "aside"],
		]);
	});

	test("an idle session gets a post at once, as a notice", () => {
		const { it, messages, handed } = recording("interactive-idle");
		it.receive(post(1, 7, { body: Array.from({ length: 50 }, () => "x").join("\n") }));
		expect(handed()).toEqual([[[1], "aside"]]);
		expect(messages[0].message.content).toContain("30 lines left out; read the full thread");
	});

	test("a critical post is never held and steers in full", () => {
		const { it, messages, handed } = recording("interactive-busy");
		const body = Array.from({ length: 50 }, () => "x").join("\n");
		it.receive(post(1, 7, { critical: true, class: "interrupt", body }));
		expect(handed()).toEqual([[[1], "steer"]]);
		expect(messages[0].message.content).toContain(body);
	});

	test("a lifecycle notice of class deferred is delivered on the next turn, not held", () => {
		const { it, handed } = recording("interactive-busy");
		it.receive(entry(1, { type: "thread_removed", threadId: 7, class: "deferred", body: undefined }));
		expect(handed()).toEqual([[[1], "nextTurn"]]);
	});

	test("held entries are returned at once by takeHeld and never handed to omp afterwards", () => {
		const { it, handed } = recording("interactive-busy");
		it.receive(post(1, 7));
		it.receive(post(2, 8));
		expect(it.takeHeld({}).map(e => e.id)).toEqual([1, 2]);
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([]);
	});

	test("a post to a thread whose notice is still held joins it, even when the session went idle meanwhile", () => {
		const { it, handed, state } = recording("interactive-busy");
		it.receive(post(1));
		state.phase = "interactive-idle";
		it.receive(post(2));
		it.receive(post(3, 8));
		expect(handed()).toEqual([[[3], "aside"]]);
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([
			[[3], "aside"],
			[[1, 2], "aside"],
		]);
	});

	test("a waiting bus_wait takes a matching post before the hold, also while the session is busy", async () => {
		const { it, handed } = recording("interactive-busy");
		const wait = it.waitNext({ threadId: 7 }, 1000);
		it.receive(post(1));
		expect((await wait).map(e => e.id)).toEqual([1]);
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([]);
	});

	test("stop cancels the held notices and ends the waiting bus_wait calls with []", async () => {
		const { it, handed } = recording("interactive-busy");
		const wait = it.waitNext({ from: "nobody" }, 60_000);
		it.receive(post(1));
		it.stop();
		expect(await wait).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(handed()).toEqual([]);
	});

	test("a notice omp refuses is warned, not thrown from the timer", () => {
		const { it, state, warnings } = recording("interactive-busy");
		it.receive(post(1));
		state.fail = true;
		jest.advanceTimersByTime(1500);
		expect(warnings).toEqual(["omp refused the message"]);
	});
});
