/**
 * What an inbox entry is about (`docs/design/arch/client.md` §3.5, §4.2-4.3): the one place that interprets its
 * `EntryType`. The envelope, the entry card and the status line read the label, never the type. Which entries are
 * posts is decided like the post notices decide it: by the thread and the class, not by the type.
 */
import { type Entry, type EntryType, parseDeliveryClass } from "../bus/entry.ts";

export interface EntryLabel {
	/** What the entry is about: a direct message, a topic, a blackboard thread or a whole blackboard. */
	readonly about: "direct" | "topic" | "thread" | "blackboard";
	/** A lifecycle notice: the topic was closed, the thread or the board removed. */
	readonly ended: boolean;
}

const LABELS: Readonly<Record<EntryType, EntryLabel>> = {
	direct_message: { about: "direct", ended: false },
	topic_message: { about: "topic", ended: false },
	topic_closed: { about: "topic", ended: true },
	thread_post: { about: "thread", ended: false },
	thread_removed: { about: "thread", ended: true },
	blackboard_removed: { about: "blackboard", ended: true },
};

/** The label of an entry. */
export function labelOf(e: Entry): EntryLabel {
	return LABELS[e.type];
}

/** Whether an entry is a post: a thread entry of a class other than deferred (lifecycle notices are deferred). */
export function isPost(e: Entry): boolean {
	return e.threadId !== undefined && parseDeliveryClass(e.class) !== "deferred";
}
