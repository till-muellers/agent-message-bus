import type { VChild } from "./h.ts";

/** How to turn a parent's children into the next ones; view/mount.ts applies it to the DOM. */
export interface PatchPlan {
	/** Per next child: the index of the previous child it patches in place, or undefined to create a node. */
	reuse: (number | undefined)[];
	/** The previous children no next child reuses, in order. */
	remove: number[];
}

/** Whether a node rendered from previous may be patched into next instead of being replaced. */
export function samePlace(previous: VChild | undefined, next: VChild): boolean {
	if (previous === undefined) return false;
	if (typeof previous === "string" || typeof next === "string") return typeof previous === typeof next;
	return previous.tag === next.tag && previous.key === next.key;
}

/**
 * Pure: each next child takes the first unused previous child in the same place (a keyed child follows its key);
 * previous holds what each existing node was rendered from, undefined for a node the patcher never rendered.
 */
export function planPatch(previous: readonly (VChild | undefined)[], next: readonly VChild[]): PatchPlan {
	const taken = new Set<number>();
	const reuse = next.map((child) => {
		const index = previous.findIndex((tree, i) => !taken.has(i) && samePlace(tree, child));
		if (index < 0) return undefined;
		taken.add(index);
		return index;
	});
	return { reuse, remove: previous.flatMap((_, i) => (taken.has(i) ? [] : [i])) };
}
