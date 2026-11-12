/** Whether the sender of an entry can get a direct reply: it is a participant of the entry's space (`client.md` §4.2). */
import { describe, expect, test } from "bun:test";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { createSenderReach } from "../../extension/lib/delivery/sender-reach.ts";
import { entry } from "../fakes.ts";

/** A sender reach over scripted participant lists; records the spaces it asked for. */
function reach(lists: Record<string, string[] | Error>) {
	const asked: string[] = [];
	const it = createSenderReach({
		participants: async space => {
			asked.push(space);
			const list = lists[space];
			if (list instanceof Error) throw list;
			return list ?? [];
		},
	});
	return { it, asked };
}

const dm = (from: string, extra: Partial<Entry> = {}) => entry(1, { space: "demo", from, ...extra });

describe("the sender reach", () => {
	test("a sender that is no participant of the space is marked as not joined", async () => {
		const { it } = reach({ demo: ["lead", "worker"] });
		expect((await it.mark(dm("admin"))).senderNotJoined).toBe(true);
		expect((await it.mark(dm("lead"))).senderNotJoined).toBeUndefined();
	});

	test("a known participant is not asked for again; an unknown sender reads the list again", async () => {
		const lists: Record<string, string[]> = { demo: ["lead"] };
		const { it, asked } = reach(lists);
		await it.mark(dm("lead"));
		await it.mark(dm("lead"));
		lists.demo = ["lead", "newcomer"];
		expect((await it.mark(dm("newcomer"))).senderNotJoined).toBeUndefined();
		expect(asked).toEqual(["demo", "demo"]);
	});

	test("topic messages are checked; thread posts and lifecycle notices name no sender to answer and are passed through", async () => {
		const { it, asked } = reach({ demo: [] });
		expect((await it.mark(dm("admin", { type: "topic_message", topic: "dev" }))).senderNotJoined).toBe(true);
		for (const type of ["thread_post", "topic_closed", "thread_removed", "blackboard_removed"] as const) {
			expect((await it.mark(dm("admin", { type, threadId: 7, topic: "dev" }))).senderNotJoined).toBeUndefined();
		}
		expect(asked).toEqual(["demo"]);
	});

	test("a failed lookup leaves the entry as it is", async () => {
		const { it } = reach({ demo: new Error("offline") });
		const e = dm("admin");
		expect(await it.mark(e)).toBe(e);
	});

	test("forget drops what was learned", async () => {
		const { it, asked } = reach({ demo: ["lead"] });
		await it.mark(dm("lead"));
		it.forget();
		await it.mark(dm("lead"));
		expect(asked).toEqual(["demo", "demo"]);
	});

	test("overlapping lookups of one space share one read", async () => {
		const { it, asked } = reach({ demo: ["lead"] });
		const marked = await Promise.all([it.mark(dm("admin")), it.mark(dm("ops")), it.mark(dm("lead"))]);
		expect(marked.map(e => e.senderNotJoined)).toEqual([true, true, undefined]);
		expect(asked).toEqual(["demo"]);
	});

	test("a read that finishes after forget does not fill the new cache", async () => {
		const answer = Promise.withResolvers<string[]>();
		const asked: string[] = [];
		const it = createSenderReach({ participants: space => (asked.push(space), asked.length === 1 ? answer.promise : Promise.resolve([])) });
		const first = it.mark(dm("lead"));
		it.forget();
		answer.resolve(["lead"]);
		await first;
		expect((await it.mark(dm("lead"))).senderNotJoined).toBe(true);
		expect(asked).toEqual(["demo", "demo"]);
	});

	test("something that is no entry is passed through", async () => {
		const { it, asked } = reach({ demo: [] });
		const broken = null as unknown as Entry;
		expect(await it.mark(broken)).toBeNull();
		expect(asked).toEqual([]);
	});
});