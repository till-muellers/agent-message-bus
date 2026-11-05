/** The inbox ledger: delivery state and acks of one connection start (`client.md` §3.3–§3.5). */
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import type { Mechanism, SessionPhase } from "../../extension/lib/delivery/delivery-class.ts";
import { createInboxLedger, type InboxLedger } from "../../extension/lib/delivery/inbox-ledger.ts";
import type { BusMessage } from "../../extension/lib/render/details.ts";
import { entry } from "../fakes.ts";

/** A ledger whose phase can change, recording every message it hands over (with its mechanism), every ack and warning. */
function ledger(initial: SessionPhase = "interactive-idle") {
	const messages: { message: BusMessage; mechanism: Mechanism }[] = [];
	const acks: number[] = [];
	const warnings: string[] = [];
	const state = { phase: initial, fail: false };
	const it = createInboxLedger({
		phase: () => state.phase,
		handOver: (message, mechanism) => {
			if (state.fail) throw new Error("omp refused the message");
			messages.push({ message, mechanism });
		},
		ack: async upTo => void acks.push(upTo),
		warn: message => void warnings.push(message),
	});
	const handed = () => messages.flatMap(m => m.message.details.entryIds);
	const notices = () => messages.map(m => [m.message.details.entryIds, m.mechanism.deliverAs]);
	return { it, messages, handed, notices, acks, state, warnings };
}

/** The stream delivers the entries with these ids. */
function stream(it: InboxLedger, ...ids: number[]): void {
	for (const id of ids) it.deliver(entry(id));
}

function post(id: number, threadId = 7, extra: Partial<Entry> = {}): Entry {
	return entry(id, { type: "thread_post", blackboard: "plan", threadId, threadTitle: "design", from: "peer", ...extra });
}

describe("the inbox ledger: ids arrive ascending and are acked once consumed in order", () => {
	test("the delivered entries are acked in order as model requests consume them", () => {
		const { it, acks } = ledger();
		stream(it, 1, 2);
		it.consumed([1]);
		it.consumed([2]);
		expect(acks).toEqual([1, 2]);
		expect(it.upTo()).toBe(2);
	});

	test("nothing is acked before the stream delivered an entry", () => {
		const { it, acks } = ledger();
		it.consumed([1]);
		expect(acks).toEqual([]);
		expect(it.upTo()).toBe(0);
	});

	test("an interrupt consumed before the older entries acks nothing until they are consumed", () => {
		const { it, acks } = ledger();
		stream(it, 1, 2);
		it.consumed([2]);
		expect(acks).toEqual([]);
		it.consumed([1]);
		expect(acks).toEqual([2]);
	});

	test("a hole holds the ack until it is filled", () => {
		const { it, acks } = ledger();
		stream(it, 1, 2, 3);
		it.consumed([1, 3]);
		it.consumed([3]);
		expect(acks).toEqual([1]);
		it.consumed([2]);
		expect(acks).toEqual([1, 3]);
	});

	test("duplicate and unknown ids change nothing", () => {
		const { it, acks } = ledger();
		stream(it, 1, 2);
		it.consumed([1, 1]);
		it.consumed([1]);
		it.consumed([99]);
		expect(acks).toEqual([1]);
		it.consumed([2]);
		expect(acks).toEqual([1, 2]);
	});

	test("an id the ascending stream skipped does not hold the ack", () => {
		const { it, acks } = ledger();
		stream(it, 1, 3);
		it.consumed([1, 3]);
		expect(acks).toEqual([3]);
	});

	test("a reconnect replays the unacked ids: omp gets each once and the ack moves once", () => {
		const { it, acks, handed } = ledger();
		stream(it, 1, 2);
		it.consumed([1]);
		stream(it, 1, 2);
		it.consumed([2]);
		expect(handed()).toEqual([1, 2]);
		expect(acks).toEqual([1, 2]);
	});

	test("one ledger per start: a new start hands the replayed entries over again, and the old start's ids hold none of its acks", () => {
		const old = ledger();
		stream(old.it, 1);
		const next = ledger();
		stream(next.it, 1, 2);
		next.it.consumed([1, 2]);
		expect(old.handed()).toEqual([1]);
		expect(next.handed()).toEqual([1, 2]);
		expect(next.acks).toEqual([2]);
		const switched = ledger();
		stream(switched.it, 5);
		switched.it.consumed([5]);
		expect(switched.acks).toEqual([5]);
	});

	test("handedUpTo resolves once every entry up to the id was handed over, false when the time ran out", async () => {
		const { it } = ledger();
		expect(await it.handedUpTo(2, 1)).toBe(false);
		const waiting = it.handedUpTo(2, 1000);
		stream(it, 1, 2);
		expect(await waiting).toBe(true);
		expect(await it.handedUpTo(1, 0)).toBe(true);
	});
});

describe("the inbox ledger: claims of a forwarded bus_wait", () => {
	test("claim returns the entries not handed over yet; the stream skips them afterwards but counts them handed over", async () => {
		const { it, handed } = ledger();
		stream(it, 1);
		expect(it.claimFromServer([entry(1), entry(2)]).map(e => e.id)).toEqual([2]);
		stream(it, 2);
		expect(await it.handedUpTo(2, 0)).toBe(true);
		stream(it, 3);
		expect(handed()).toEqual([1, 3]);
	});

	test("a claimed id is acked as soon as this start's stream delivers it, never before", () => {
		const { it, acks } = ledger();
		stream(it, 1);
		it.consumed([1]);
		it.claimFromServer([entry(2)]);
		expect(acks).toEqual([1]);
		stream(it, 2);
		expect(acks).toEqual([1, 2]);
	});

	test("a replayed id omp got is not acked on arrival; a claimed one waits for the older ids", () => {
		const { it, acks, handed } = ledger();
		stream(it, 1);
		it.claimFromServer([entry(2)]);
		stream(it, 1, 2);
		expect(handed()).toEqual([1]);
		expect(acks).toEqual([]);
		it.consumed([1]);
		expect(acks).toEqual([2]);
	});
});

describe("the inbox ledger: waiting bus_wait calls", () => {
	test("a waiter gets the next entry; omp does not, and it counts as consumed", async () => {
		const { it, handed, acks } = ledger();
		const wait = it.awaitEntry({}, 1000);
		stream(it, 1);
		expect((await wait).map(e => e.id)).toEqual([1]);
		expect(handed()).toEqual([]);
		expect(acks).toEqual([1]);
	});

	test("an entry the waiter does not want is handed over the normal way", async () => {
		const { it, handed } = ledger();
		const fromLead = it.awaitEntry({ from: "lead" }, 1000);
		it.deliver(entry(1, { from: "peer" }));
		it.deliver(entry(2, { from: "lead" }));
		expect((await fromLead).map(e => e.id)).toEqual([2]);
		expect(handed()).toEqual([1]);
	});

	test("a waiter can wait for one thread or for ids after a given one", async () => {
		const { it, handed } = ledger();
		const thread = it.awaitEntry({ threadId: 7 }, 1000);
		const after = it.awaitEntry({ after: 3 }, 1000);
		it.deliver(entry(2, { type: "thread_post", threadId: 6 }));
		it.deliver(entry(3, { type: "thread_post", threadId: 7 }));
		stream(it, 4);
		expect((await thread).map(e => e.id)).toEqual([3]);
		expect((await after).map(e => e.id)).toEqual([4]);
		expect(handed()).toEqual([2]);
	});

	test("a wait that runs out returns [] and the entry after it goes to omp", async () => {
		const { it, handed, acks } = ledger();
		expect(await it.awaitEntry({}, 1)).toEqual([]);
		stream(it, 1);
		expect(handed()).toEqual([1]);
		expect(acks).toEqual([]);
	});

	test("an aborted wait returns [] and takes nothing", async () => {
		const { it, handed } = ledger();
		const controller = new AbortController();
		const wait = it.awaitEntry({}, 60_000, controller.signal);
		controller.abort();
		expect(await wait).toEqual([]);
		stream(it, 1);
		expect(handed()).toEqual([1]);
		const aborted = new AbortController();
		aborted.abort();
		expect(await it.awaitEntry({}, 60_000, aborted.signal)).toEqual([]);
	});

	test("an entry already handed to omp is never returned, not even when a reconnect replays it", async () => {
		const { it, handed } = ledger();
		stream(it, 1);
		const wait = it.awaitEntry({}, 1);
		stream(it, 1);
		expect(await wait).toEqual([]);
		expect(handed()).toEqual([1]);
	});

	test("an entry a waiter took counts as handed over, so a replay does not reach omp", async () => {
		const { it, handed } = ledger();
		const wait = it.awaitEntry({}, 1000);
		stream(it, 1);
		await wait;
		stream(it, 1);
		expect(handed()).toEqual([]);
		expect(await it.handedUpTo(1, 0)).toBe(true);
	});
});

describe("the inbox ledger: blackboard notices", () => {
	beforeEach(() => jest.useFakeTimers());
	afterEach(() => jest.useRealTimers());

	test("while busy, 3 posts to one thread within the window become one aside with 3 entryIds, acked together", () => {
		const { it, messages, notices, acks } = ledger("interactive-busy");
		for (const id of [1, 2, 3]) it.deliver(post(id));
		expect(notices()).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([[[1, 2, 3], "aside"]]);
		expect(messages[0].message.content.match(/<agent-bus-entry /g)?.length).toBe(3);
		it.consumed(messages[0].message.details.entryIds);
		expect(acks).toEqual([3]);
	});

	test("posts to different threads stay separate notices", () => {
		const { it, notices } = ledger("interactive-busy");
		it.deliver(post(1, 7));
		it.deliver(post(2, 8));
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([
			[[1], "aside"],
			[[2], "aside"],
		]);
	});

	test("an idle session gets a post at once, as a notice", () => {
		const { it, messages, notices } = ledger("interactive-idle");
		it.deliver(post(1, 7, { body: Array.from({ length: 50 }, () => "x").join("\n") }));
		expect(notices()).toEqual([[[1], "aside"]]);
		expect(messages[0].message.content).toContain("30 lines left out; read the full thread");
	});

	test("a critical post is never held and steers in full", () => {
		const { it, messages, notices } = ledger("interactive-busy");
		const body = Array.from({ length: 50 }, () => "x").join("\n");
		it.deliver(post(1, 7, { critical: true, class: "interrupt", body }));
		expect(notices()).toEqual([[[1], "steer"]]);
		expect(messages[0].message.content).toContain(body);
	});

	test("a lifecycle notice of class deferred is delivered on the next turn, not held", () => {
		const { it, notices } = ledger("interactive-busy");
		it.deliver(entry(1, { type: "thread_removed", threadId: 7, class: "deferred", body: undefined }));
		expect(notices()).toEqual([[[1], "nextTurn"]]);
	});

	test("held entries are returned at once by takeHeld, consumed, and never handed to omp afterwards", () => {
		const { it, notices, acks } = ledger("interactive-busy");
		it.deliver(post(1, 7));
		it.deliver(post(2, 8));
		expect(it.takeHeld({}).map(e => e.id)).toEqual([1, 2]);
		expect(acks).toEqual([2]);
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([]);
	});

	test("a post to a thread whose notice is still held joins it, even when the session went idle meanwhile", () => {
		const { it, notices, state } = ledger("interactive-busy");
		it.deliver(post(1));
		state.phase = "interactive-idle";
		it.deliver(post(2));
		it.deliver(post(3, 8));
		expect(notices()).toEqual([[[3], "aside"]]);
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([
			[[3], "aside"],
			[[1, 2], "aside"],
		]);
	});

	test("a waiting bus_wait takes a matching post before the hold, also while the session is busy", async () => {
		const { it, notices } = ledger("interactive-busy");
		const wait = it.awaitEntry({ threadId: 7 }, 1000);
		it.deliver(post(1));
		expect((await wait).map(e => e.id)).toEqual([1]);
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([]);
	});

	test("stop drops the held notices unhanded and ends the waiting bus_wait calls with []", async () => {
		const { it, notices, acks } = ledger("interactive-busy");
		const wait = it.awaitEntry({ from: "nobody" }, 60_000);
		it.deliver(post(1));
		it.stop();
		expect(await wait).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(notices()).toEqual([]);
		expect(acks).toEqual([]);
	});

	test("a notice omp refuses is warned, not thrown from the timer", () => {
		const { it, state, warnings } = ledger("interactive-busy");
		it.deliver(post(1));
		state.fail = true;
		jest.advanceTimersByTime(1500);
		expect(warnings).toEqual(["omp refused the message"]);
	});
});
