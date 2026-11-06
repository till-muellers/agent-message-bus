import { type ChildArg, h, type VNode } from "../view/h.ts";

export interface Row {
	key: string;
	cells: readonly ChildArg[];
}

/**
 * A table with a header row; rows are keyed so polling re-renders patch them in place.
 * It sits in a box that scrolls sideways, so a name wider than the page widens the box, not the page.
 */
export function table(head: readonly string[], rows: readonly Row[]): VNode {
	return h(
		"div",
		{ class: "scroll-x" },
		h(
			"table",
			null,
			h("thead", null, h("tr", { key: "head" }, head.map((title) => h("th", null, title)))),
			h("tbody", null, rows.map((row) => h("tr", { key: row.key }, row.cells.map((cell) => h("td", null, cell))))),
		),
	);
}
