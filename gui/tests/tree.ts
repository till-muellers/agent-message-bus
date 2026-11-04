import type { VChild, VNode } from "../src/view/h.ts";

/** All text of a tree, depth first, joined by single spaces. */
export function textOf(node: VChild): string {
	if (typeof node === "string") return node;
	return node.children.map(textOf).join(" ");
}

/** Every node of the tree (the root included) for which match holds. */
export function findAll(node: VChild, match: (n: VNode) => boolean): VNode[] {
	if (typeof node === "string") return [];
	const own = match(node) ? [node] : [];
	return own.concat(node.children.flatMap((child) => findAll(child, match)));
}

export function byTag(tag: string): (n: VNode) => boolean {
	return (n) => n.tag === tag;
}
