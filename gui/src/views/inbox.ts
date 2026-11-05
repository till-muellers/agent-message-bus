import type { Entry, Identity } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { table } from "./table.ts";
import { time } from "./time.ts";

/** What an entry concerns: a live thread as a link, a removed thread, a topic or a blackboard as text. */
function subject(e: Entry): ChildArg {
	if (e.threadId !== undefined && e.type !== "thread_removed") {
		const href = hrefOf({ name: "thread", space: e.space, threadId: e.threadId });
		return h("a", { href }, e.threadTitle ?? `#${e.threadId}`);
	}
	return e.threadTitle ?? e.topic ?? e.blackboard;
}

/** The entry kind as words ("thread post"), critical first, with what it concerns. */
function kind(e: Entry): ChildArg[] {
	const label = `${e.critical ? "critical " : ""}${e.type.replaceAll("_", " ")}`;
	const about = subject(e);
	return about === undefined ? [label] : [`${label} on `, about];
}

function entries(list: readonly Entry[], limit: number): ChildArg {
	if (list.length === 0) return h("p", null, "No entries.");
	const rows = list.map((e) => ({ key: String(e.id), cells: [time(e.createdAt), e.space, e.from, kind(e), e.body ?? ""] }));
	const cut = list.length >= limit ? h("p", null, `Showing the oldest ${limit} entries; newer ones are not shown.`) : null;
	return [table(["Time", "Space", "From", "Kind", "Message"], rows), cut];
}

/** The signed-in agent's own inbox, read-only: no acknowledge, entries stay for the agent session; limit is the server's read limit. */
export function inboxView(identity: Identity, list: readonly Entry[], limit: number): VNode {
	return h("section", { key: "inbox" }, h("h1", null, `Inbox of ${identity.kind} ${identity.name}`), entries(list, limit));
}
