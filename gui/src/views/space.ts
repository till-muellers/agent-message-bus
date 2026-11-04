import type { Overview, Participant, PendingInvite, SpaceDetail } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import type { PendingConfirm } from "../state/app-state.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { type FormIntents, inviteForm, sendForm } from "./forms.ts";
import { type Row, table } from "./table.ts";
import { time } from "./time.ts";

export interface SpaceViewIntents extends Pick<FormIntents, "invite" | "send"> {
	ask(confirm: PendingConfirm): void;
	cancelInvite(space: string, invite: PendingInvite): void;
}

/** What the page offers: lead is true with leader rights in the space (leader of it or admin). */
interface Page {
	space: string;
	lead: boolean;
	intents: SpaceViewIntents;
}

/** A titled section with its table (or a sentence when the list is empty) and what follows it. */
function section(key: string, title: string, empty: string, head: readonly string[], rows: readonly Row[], ...more: ChildArg[]): VNode {
	const body: ChildArg = rows.length === 0 ? h("p", null, empty) : table(head, rows);
	return h("section", { key }, h("h2", null, title), body, more);
}

function button(key: string, label: string, click: () => void): VNode {
	return h("button", { type: "button", key, on: { click } }, label);
}

/** Admins cannot be revoked from a space; leaders and members can, after a confirmation. */
function revokeButton(page: Page, p: Participant): ChildArg {
	if (!page.lead || p.role === "admin") return null;
	return button(`revoke-${p.name}`, "Revoke", () => page.intents.ask({ kind: "revoke", space: page.space, name: p.name, role: p.role }));
}

function participants(o: Overview, page: Page): VNode {
	const rows = o.participants.map((p) => ({ key: p.name, cells: [p.name, p.role, p.connected ? "yes" : "no", revokeButton(page, p)] }));
	return section("participants", "Participants", "No participants.", ["Name", "Role", "Connected", ""], rows);
}

function topics(o: Overview, page: Page): VNode {
	const rows = o.topics.map((t) => ({ key: t.name, cells: [t.name, t.createdBy] }));
	const form = sendForm(o.space, o.topics.map((t) => t.name), page.intents);
	return section("topics", "Topics", "No topics.", ["Topic", "Created by"], rows, form);
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

function invites(pending: readonly PendingInvite[], page: Page): VNode {
	const rows = pending.map((i) => {
		const cancel = button(`cancel-${i.id}`, "Cancel invite", () => page.intents.cancelInvite(page.space, i));
		return { key: i.id, cells: [i.name, i.invitedBy, time(i.expiresAt), cancel] };
	});
	const head = ["Name", "Invited by", "Expires", ""];
	return section("invites", "Open invites", "No open invites.", head, rows, inviteForm(page.space, page.intents));
}

function deletion(page: Page): VNode {
	const ask = (): void => page.intents.ask({ kind: "delete-space", space: page.space });
	return h("section", { key: "delete" }, h("h2", null, "Delete the space"), button("delete-space", "Delete space", ask));
}

/** A space: participants, topics with the send form and, with leader rights, invites, revoke and delete. */
export function spaceView(detail: SpaceDetail, intents: SpaceViewIntents): VNode {
	const o = detail.overview;
	const page: Page = { space: o.space, lead: detail.invites !== undefined, intents };
	return h(
		"div",
		{ key: `space-${o.space}` },
		h("h1", null, o.space),
		participants(o, page),
		topics(o, page),
		blackboards(o),
		recentThreads(o),
		detail.invites === undefined ? null : [invites(detail.invites, page), deletion(page)],
	);
}
