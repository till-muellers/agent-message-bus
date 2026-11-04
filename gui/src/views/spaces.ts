import type { SpaceList, SpaceSummary } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { h, type VNode } from "../view/h.ts";
import { type Row, table } from "./table.ts";

function row(space: SpaceSummary, withInvites: boolean): Row {
	const link = h("a", { href: hrefOf({ name: "space", space: space.name }) }, space.name);
	const cells = [link, space.leaders.length, space.admins.join(", "), space.members];
	return { key: space.name, cells: withInvites ? [...cells, space.invites ?? ""] : cells };
}

/** Every space the session may see, in the server's order; open invites only with leader rights. */
export function spacesView(list: SpaceList): VNode {
	if (list.spaces.length === 0) return h("p", null, "No spaces yet.");
	const withInvites = list.spaces.some((space) => space.invites !== undefined);
	const head = ["Space", "Leaders", "Admins", "Members", ...(withInvites ? ["Open invites"] : [])];
	const rows = list.spaces.map((space) => row(space, withInvites));
	return h("section", { key: "spaces" }, h("h1", null, "Spaces"), table(head, rows));
}
