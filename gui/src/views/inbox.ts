import type { Entry, Identity } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { table } from "./table.ts";
import { time } from "./time.ts";

/** The entry kind as words ("thread post"), critical first, with the topic or thread it concerns. */
function kind(e: Entry): ChildArg[] {
	const label = `${e.critical ? "critical " : ""}${e.type.replaceAll("_", " ")}`;
	if (e.threadId !== undefined) {
		const link = h("a", { href: hrefOf({ name: "thread", space: e.space, threadId: e.threadId }) }, e.threadTitle ?? `#${e.threadId}`);
		return [`${label} on`, link];
	}
	return [e.topic === undefined ? label : `${label} on ${e.topic}`];
}

function entries(list: readonly Entry[]): ChildArg {
	if (list.length === 0) return h("p", null, "No entries.");
	const rows = list.map((e) => ({ key: String(e.id), cells: [time(e.createdAt), e.space, e.from, kind(e), e.body ?? ""] }));
	return table(["Time", "Space", "From", "Kind", "Message"], rows);
}

/** The signed-in agent's own inbox, read-only: no acknowledge, entries stay for the agent session. */
export function inboxView(identity: Identity, list: readonly Entry[]): VNode {
	return h("section", { key: "inbox" }, h("h1", null, `Inbox of ${identity.kind} ${identity.name}`), entries(list));
}
