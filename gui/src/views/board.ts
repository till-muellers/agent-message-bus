import type { ThreadInfo } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { type ChildArg, h, type VNode } from "../view/h.ts";
import { table } from "./table.ts";
import { time } from "./time.ts";

function threads(space: string, list: readonly ThreadInfo[]): ChildArg {
	if (list.length === 0) return h("p", null, "No threads.");
	const rows = list.map((t) => {
		const link = h("a", { href: hrefOf({ name: "thread", space, threadId: t.id }) }, t.title);
		return { key: String(t.id), cells: [t.id, link, t.createdBy, t.posts, time(t.lastPostAt)] };
	});
	return table(["Id", "Thread", "Created by", "Posts", "Last post"], rows);
}

/** The threads of a blackboard in the server's order (most recently active first). */
export function boardView(space: string, board: string, list: readonly ThreadInfo[]): VNode {
	return h(
		"section",
		{ key: `board-${space}-${board}` },
		h("p", null, h("a", { href: hrefOf({ name: "space", space }) }, space)),
		h("h1", null, board),
		threads(space, list),
	);
}
