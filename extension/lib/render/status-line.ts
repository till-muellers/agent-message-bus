/**
 * Blackboard activity in omp's status line (`docs/design/arch/client.md` §3.5, `omp-blackboards.feature` rule 4):
 * the posts handed over to the session are counted per thread, "bus: 2 new posts in plan/7"; a post is told by its
 * thread and class like a post notice (`isPost`). A successful `bus_read_thread` or the thread's removal clears its
 * thread, a board's removal its threads, logout and a restart clear them all.
 */
import type { Entry } from "../bus/entry.ts";
import { isPost, labelOf } from "./entry-label.ts";

/** The status line key of the extension (`ctx.ui.setStatus`). */
export const STATUS_KEY = "agent-bus";

export interface UnreadPosts {
	/** Counts the thread posts among the entries handed over to the session; removals drop their counts. */
	add(entries: readonly Entry[]): void;
	/** The thread was read: its count goes. */
	read(space: string, threadId: number): void;
	/** Logout or restart: every count goes, and with them the bus entry. */
	clear(): void;
}

interface Thread {
	space: string;
	blackboard?: string;
	threadId?: number;
	count: number;
}

const threadKey = (space: string, threadId: number | undefined): string => JSON.stringify([space, threadId]);

function statusText(threads: Iterable<Thread>): string | undefined {
	const parts = [...threads].map(t => `${t.count} new ${t.count === 1 ? "post" : "posts"} in ${t.blackboard}/${t.threadId}`);
	return parts.length > 0 ? `bus: ${parts.join(" · ")}` : undefined;
}

/** Drops the counts of the threads on the removed board; true when there were any. */
function removeBoard(threads: Map<string, Thread>, removed: Entry): boolean {
	const keys = [...threads].filter(([, t]) => t.space === removed.space && t.blackboard === removed.blackboard).map(([key]) => key);
	for (const key of keys) threads.delete(key);
	return keys.length > 0;
}

/** The unread posts of the session; `show` sets omp's status entry (`undefined` removes it). */
export function createUnreadPosts(show: (key: string, text: string | undefined) => void): UnreadPosts {
	const threads = new Map<string, Thread>();
	const update = () => show(STATUS_KEY, statusText(threads.values()));
	/** Applies one entry; true when a count changed. */
	const apply = (e: Entry): boolean => {
		const key = threadKey(e.space, e.threadId);
		const { about, ended } = labelOf(e);
		if (about === "blackboard") return removeBoard(threads, e);
		if (about === "thread" && ended) return threads.delete(key);
		if (!isPost(e)) return false;
		const count = (threads.get(key)?.count ?? 0) + 1;
		threads.set(key, { space: e.space, blackboard: e.blackboard, threadId: e.threadId, count });
		return true;
	};
	return {
		add(entries) {
			// Every entry is applied; the status changes when one of them changed a count.
			if (entries.map(apply).includes(true)) update();
		},
		read(space, threadId) {
			if (threads.delete(threadKey(space, threadId))) update();
		},
		clear() {
			threads.clear();
			update();
		},
	};
}
