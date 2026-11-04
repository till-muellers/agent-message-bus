/** The hold of thread entries while the session is busy (`client.md` §3.5, `omp-blackboards.feature` rule 2). */
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { createPostNotices, isPostNotice } from "../../extension/lib/delivery/post-notices.ts";
import { entry } from "../fakes.ts";

function post(id: number, threadId = 7, extra: Partial<Entry> = {}): Entry {
	return entry(id, { type: "thread_post", blackboard: "plan", threadId, threadTitle: "design", from: "peer", ...extra });
}

/** A hold with a short window that records every group it flushes. */
function hold(windowMs = 1500) {
	const flushed: number[][] = [];
	const it = createPostNotices({ windowMs, flush: entries => void flushed.push(entries.map(e => e.id)) });
	return { it, flushed };
}

describe("post notices", () => {
	beforeEach(() => jest.useFakeTimers());
	afterEach(() => jest.useRealTimers());

	test("a queue-class entry of a thread is a post notice; critical, deferred and other entries are not", () => {
		expect(isPostNotice(post(1))).toBe(true);
		expect(isPostNotice(post(2, 7, { class: "interrupt", critical: true }))).toBe(false);
		expect(isPostNotice(entry(3, { type: "thread_removed", threadId: 7, class: "deferred" }))).toBe(false);
		expect(isPostNotice(entry(4))).toBe(false);
	});

	test("posts to one thread within the window are flushed together, once", () => {
		const { it, flushed } = hold();
		for (const id of [1, 2, 3]) it.hold(post(id));
		jest.advanceTimersByTime(1499);
		expect(flushed).toEqual([]);
		jest.advanceTimersByTime(1);
		expect(flushed).toEqual([[1, 2, 3]]);
	});

	test("posts to different threads, boards or spaces are flushed separately", () => {
		const { it, flushed } = hold();
		it.hold(post(1, 7));
		it.hold(post(2, 8));
		it.hold(post(3, 7, { blackboard: "other" }));
		it.hold(post(4, 7, { space: "beta" }));
		it.hold(post(5, 7));
		jest.advanceTimersByTime(1500);
		expect(flushed.toSorted((a, b) => a[0] - b[0])).toEqual([[1, 5], [2], [3], [4]]);
	});

	test("take returns the held entries a wait filter matches, and they are never flushed", () => {
		const { it, flushed } = hold();
		it.hold(post(1, 7));
		it.hold(post(2, 8));
		it.hold(post(3, 7, { from: "lead" }));
		expect(it.take({ threadId: 7 }).map(e => e.id)).toEqual([1, 3]);
		expect(it.take({ threadId: 7 })).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(flushed).toEqual([[2]]);
	});

	test("take keeps the id order across threads and leaves what the filter does not match", () => {
		const { it, flushed } = hold();
		it.hold(post(1, 8));
		it.hold(post(2, 7));
		it.hold(post(3, 8, { from: "lead" }));
		expect(it.take({ after: 1 }).map(e => e.id)).toEqual([2, 3]);
		expect(it.take({ from: "nobody" })).toEqual([]);
		jest.advanceTimersByTime(1500);
		expect(flushed).toEqual([[1]]);
	});
});
