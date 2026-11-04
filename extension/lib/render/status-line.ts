/**
 * Blackboard activity in omp's status line (`docs/design/arch/client.md` §3.5, `omp-blackboards.feature` rule 4):
 * the posts handed over to the session are counted per thread, "bus: 2 new posts in plan/7"; a successful
 * `bus_read_thread` clears its thread, logout clears them all.
 */
import type { Entry } from "../bus/entry.ts";

/** The status line key of the extension (`ctx.ui.setStatus`). */
export const STATUS_KEY = "agent-bus";

export interface UnreadPosts {
	/** Counts the thread posts among the entries handed over to the session. */
	add(entries: readonly Entry[]): void;
	/** The thread was read: its count goes. */
	read(space: string, threadId: number): void;
	/** Logout: every count goes, and with them the bus entry. */
	clear(): void;
}

interface Thread {
	label: string;
	count: number;
}

const threadKey = (space: string, threadId: number | undefined): string => JSON.stringify([space, threadId]);

function statusText(threads: Iterable<Thread>): string | undefined {
	const parts = [...threads].map(({ label, count }) => `${count} new ${count === 1 ? "post" : "posts"} in ${label}`);
	return parts.length > 0 ? `bus: ${parts.join(" · ")}` : undefined;
}

/** The unread posts of the session; `show` sets omp's status entry (`undefined` removes it). */
export function createUnreadPosts(show: (key: string, text: string | undefined) => void): UnreadPosts {
	const threads = new Map<string, Thread>();
	const update = () => show(STATUS_KEY, statusText(threads.values()));
	return {
		add(entries) {
			const posts = entries.filter(e => e.type === "thread_post");
			for (const e of posts) {
				const key = threadKey(e.space, e.threadId);
				const thread = threads.get(key) ?? { label: `${e.blackboard}/${e.threadId}`, count: 0 };
				threads.set(key, { ...thread, count: thread.count + 1 });
			}
			if (posts.length > 0) update();
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
