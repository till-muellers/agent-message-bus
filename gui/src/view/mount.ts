import type { Handlers, VChild, VNode } from "./h.ts";

/** The tree each DOM node was last rendered from. */
const rendered = new WeakMap<Node, VChild>();

/** Whether a node rendered from previous may be patched into next instead of being replaced. */
export function samePlace(previous: VChild | undefined, next: VChild): boolean {
	if (previous === undefined) return false;
	if (typeof previous === "string" || typeof next === "string") return typeof previous === typeof next;
	return previous.tag === next.tag && previous.key === next.key;
}

/** Renders tree into root, reusing the nodes of the last render (focus and typed input survive). */
export function mount(root: Element, tree: VChild): void {
	patchChildren(root, [tree]);
}

/** Matches first and removes the leftovers before placing, so removals move no kept node (a moved node loses focus). */
function patchChildren(parent: Node, next: readonly VChild[]): void {
	const pool = Array.from(parent.childNodes);
	const nodes = next.map((child) => {
		const reuse = takeMatch(pool, child);
		return reuse ? patch(reuse, child) : create(child);
	});
	for (const stale of pool) parent.removeChild(stale);
	nodes.forEach((node, index) => {
		const at = parent.childNodes[index] ?? null;
		if (at !== node) parent.insertBefore(node, at);
	});
}

function takeMatch(pool: Node[], child: VChild): Node | undefined {
	const index = pool.findIndex((node) => samePlace(rendered.get(node), child));
	return index < 0 ? undefined : pool.splice(index, 1)[0];
}

function create(child: VChild): Node {
	const node = typeof child === "string" ? document.createTextNode(child) : createElement(child);
	rendered.set(node, child);
	return node;
}

function createElement(vnode: VNode): Element {
	const el = document.createElement(vnode.tag);
	patchElement(el, { attrs: {}, on: {} }, vnode);
	return el;
}

function patch(node: Node, next: VChild): Node {
	const previous = rendered.get(node);
	if (typeof next === "string") {
		if (node.nodeValue !== next) node.nodeValue = next;
	} else if (node instanceof Element && previous !== undefined && typeof previous !== "string") {
		patchElement(node, previous, next);
	}
	rendered.set(node, next);
	return node;
}

function patchElement(el: Element, previous: Pick<VNode, "attrs" | "on">, next: VNode): void {
	patchAttrs(el, previous.attrs, next.attrs);
	patchListeners(el, previous.on, next.on);
	patchChildren(el, next.children);
}

function patchAttrs(el: Element, previous: VNode["attrs"], next: VNode["attrs"]): void {
	for (const name of Object.keys(previous)) if (!(name in next)) el.removeAttribute(name);
	for (const [name, value] of Object.entries(next)) if (el.getAttribute(name) !== value) el.setAttribute(name, value);
}

function patchListeners(el: Element, previous: Handlers, next: Handlers): void {
	for (const [type, handler] of Object.entries(previous)) el.removeEventListener(type, handler);
	for (const [type, handler] of Object.entries(next)) el.addEventListener(type, handler);
}
