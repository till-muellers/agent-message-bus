/**
 * Blackboard notices (`docs/design/arch/client.md` §3.5, `omp-blackboards.feature` rules 1-2): while the
 * session is busy, the dispatcher holds the entries of a thread whose class may be held, so posts that
 * arrive within a short window reach omp as one notice. Held entries are what `bus_wait` can take.
 * The grouping key is the entry's thread fields, never its type.
 */
import { type Entry, parseDeliveryClass } from "../bus/entry.ts";
import { holdable } from "./delivery-class.ts";
import { matches, type WaitFilter } from "./waiters.ts";

/** How long the first held entry of a thread waits for more (a debounce, not a step-boundary sync). */
export const NOTICE_WINDOW_MS = 1500;

/** Whether an entry is delivered as a post notice: a thread entry of a class the dispatcher may hold. */
export function isPostNotice(entry: Entry): boolean {
	return entry.threadId !== undefined && holdable(parseDeliveryClass(entry.class));
}

export interface PostNotices {
	/** Holds a post notice; the first one of its thread starts the window. */
	hold(entry: Entry): void;
	/** Removes and returns the held entries `filter` matches, in id order; they are never flushed. */
	take(filter: WaitFilter): Entry[];
}

export interface PostNoticesDeps {
	windowMs: number;
	/** Hands the held entries of one thread over, once their window ran out. */
	flush(entries: Entry[]): void;
}

interface Group {
	entries: Entry[];
	timer: ReturnType<typeof setTimeout>;
}

function threadKey(e: Entry): string {
	return JSON.stringify([e.space, e.blackboard, e.threadId]);
}

export function createPostNotices(deps: PostNoticesDeps): PostNotices {
	const groups = new Map<string, Group>();
	const start = (key: string, entry: Entry): Group => ({
		entries: [entry],
		timer: setTimeout(() => {
			const group = groups.get(key);
			groups.delete(key);
			if (group) deps.flush(group.entries);
		}, deps.windowMs),
	});
	const takeFrom = (key: string, group: Group, filter: WaitFilter): Entry[] => {
		const taken = group.entries.filter(e => matches(filter, e));
		group.entries = group.entries.filter(e => !matches(filter, e));
		if (group.entries.length === 0) {
			clearTimeout(group.timer);
			groups.delete(key);
		}
		return taken;
	};
	return {
		hold(entry) {
			const key = threadKey(entry);
			const group = groups.get(key);
			if (group) group.entries.push(entry);
			else groups.set(key, start(key, entry));
		},
		take(filter) {
			const taken = [...groups].flatMap(([key, group]) => takeFrom(key, group, filter));
			return taken.toSorted((a, b) => a.id - b.id);
		},
	};
}
