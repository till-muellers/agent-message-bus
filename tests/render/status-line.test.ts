/** The unread posts per thread in the status line (`client.md` §3.5, `omp-blackboards.feature` rule 4). */
import { describe, expect, test } from "bun:test";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { createUnreadPosts, STATUS_KEY } from "../../extension/lib/render/status-line.ts";
import { entry } from "../fakes.ts";

function post(id: number, threadId = 7, extra: Partial<Entry> = {}): Entry {
	return entry(id, { type: "thread_post", blackboard: "plan", threadId, threadTitle: "design", ...extra });
}

/** Unread posts that record every status they show. */
function unread() {
	const shown: (string | undefined)[] = [];
	const it = createUnreadPosts((key, text) => {
		expect(key).toBe(STATUS_KEY);
		shown.push(text);
	});
	return { it, shown, last: () => shown.at(-1) };
}

describe("the status line", () => {
	test("the key is agent-bus", () => {
		expect(STATUS_KEY).toBe("agent-bus");
	});

	test("unread posts are counted per thread", () => {
		const { it, last } = unread();
		it.add([post(1)]);
		expect(last()).toBe("bus: 1 new post in plan/7");
		it.add([post(2)]);
		expect(last()).toBe("bus: 2 new posts in plan/7");
		it.add([post(3, 8), post(4, 8)]);
		expect(last()).toBe("bus: 2 new posts in plan/7 · 2 new posts in plan/8");
	});

	test("entries that are not thread posts are not counted and change nothing", () => {
		const { it, shown } = unread();
		it.add([entry(1), entry(2, { type: "thread_removed", threadId: 7, blackboard: "plan" })]);
		expect(shown).toEqual([]);
	});

	test("reading a thread clears its count; the last one read clears the bus entry", () => {
		const { it, last } = unread();
		it.add([post(1), post(2, 8)]);
		it.read("demo", 7);
		expect(last()).toBe("bus: 1 new post in plan/8");
		it.read("demo", 8);
		expect(last()).toBeUndefined();
	});

	test("reading a thread of another space or one without posts changes nothing", () => {
		const { it, shown } = unread();
		it.add([post(1)]);
		it.read("other", 7);
		it.read("demo", 9);
		expect(shown).toEqual(["bus: 1 new post in plan/7"]);
	});

	test("clear removes the bus entry", () => {
		const { it, last } = unread();
		it.add([post(1)]);
		it.clear();
		expect(last()).toBeUndefined();
		it.add([post(2)]);
		expect(last()).toBe("bus: 1 new post in plan/7");
	});
});
