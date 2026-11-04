import type { Overview, PendingInvite, SpaceDetail } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { type Row, table } from "./table.ts";
import { time } from "./time.ts";

/** A titled section with its table, or a sentence when the list is empty. */
function section(key: string, title: string, empty: string, head: readonly string[], rows: readonly Row[]): VNode {
	const body: ChildArg = rows.length === 0 ? h("p", null, empty) : table(head, rows);
	return h("section", { key }, h("h2", null, title), body);
}

function participants(o: Overview): VNode {
	const rows = o.participants.map((p) => ({ key: p.name, cells: [p.name, p.role, p.connected ? "yes" : "no"] }));
	return section("participants", "Participants", "No participants.", ["Name", "Role", "Connected"], rows);
}

function topics(o: Overview): VNode {
	const rows = o.topics.map((t) => ({ key: t.name, cells: [t.name, t.createdBy] }));
	return section("topics", "Topics", "No topics.", ["Topic", "Created by"], rows);
}

function blackboards(o: Overview): VNode {
	const rows = o.blackboards.map((b) => {
		const link = h("a", { href: hrefOf({ name: "board", space: o.space, board: b.name }) }, b.name);
		return { key: b.name, cells: [link, b.owner, b.threads] };
	});
	return section("blackboards", "Blackboards", "No blackboards.", ["Blackboard", "Owner", "Threads"], rows);
}

function recentThreads(o: Overview): VNode {
	const rows = o.recentThreads.map((t) => {
		const link = h("a", { href: hrefOf({ name: "thread", space: o.space, threadId: t.id }) }, t.title);
		return { key: String(t.id), cells: [link, t.blackboard, t.posts, time(t.lastPostAt)] };
	});
	return section("threads", "Recent threads", "No threads.", ["Thread", "Blackboard", "Posts", "Last post"], rows);
}

function invites(pending: readonly PendingInvite[]): VNode {
	const rows = pending.map((i) => ({ key: i.id, cells: [i.name, i.invitedBy, time(i.expiresAt)] }));
	return section("invites", "Open invites", "No open invites.", ["Name", "Invited by", "Expires"], rows);
}

/** A space: participants, topics, blackboards, recent threads and, with leader rights, the open invites. */
export function spaceView(detail: SpaceDetail): VNode {
	const o = detail.overview;
	return h(
		"div",
		{ key: `space-${o.space}` },
		h("h1", null, o.space),
		participants(o),
		topics(o),
		blackboards(o),
		recentThreads(o),
		detail.invites === undefined ? null : invites(detail.invites),
	);
}
