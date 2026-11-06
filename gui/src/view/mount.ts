import type { Handlers, VChild, VNode } from "./h.ts";
import { planPatch } from "./plan-patch.ts";

/** The tree each DOM node was last rendered from. */
const rendered = new WeakMap<Node, VChild>();

/** Where the focus was when an `autofocus` node took it: it goes back there when the node is removed with the focus inside. */
const focusReturn = new WeakMap<Node, HTMLElement>();

/** Renders tree into root, reusing the nodes of the last render (focus and typed input survive). */
export function mount(root: Element, tree: VChild): void {
	patchChildren(root, [tree]);
}

/** Applies planPatch: removes the leftovers before placing, so removals move no kept node (a moved node loses focus). */
function patchChildren(parent: Node, next: readonly VChild[]): void {
	const existing = Array.from(parent.childNodes);
	const plan = planPatch(existing.map((node) => rendered.get(node)), next);
	const nodes = next.map((child, i) => {
		const index = plan.reuse[i];
		const reused = index === undefined ? undefined : existing[index];
		return reused ? patch(reused, child) : create(child);
	});
	for (const index of plan.remove) removeNode(existing[index]);
	nodes.forEach((node, index) => {
		const at = parent.childNodes[index] ?? null;
		if (at !== node) parent.insertBefore(node, at);
	});
}

function create(child: VChild): Node {
	const node = typeof child === "string" ? document.createTextNode(child) : createElement(child);
	rendered.set(node, child);
	return node;
}

function createElement(vnode: VNode): Element {
	const el = document.createElement(vnode.tag);
	patchElement(el, { attrs: {}, on: {} }, vnode);
	if ("autofocus" in vnode.attrs) queueMicrotask(() => takeFocus(el));
	return el;
}

/** Moves the focus to a node that has just appeared (an `autofocus` node: a confirmation or a secret, which must not be missed). */
function takeFocus(el: Element): void {
	if (!(el instanceof HTMLElement) || !el.isConnected) return;
	const before = document.activeElement;
	if (before instanceof HTMLElement && before !== document.body) focusReturn.set(el, before);
	el.focus();
}

/** Removes a node; the focus it held goes back to where it was before the node took it, unless that is gone too. */
function removeNode(node: ChildNode | undefined): void {
	if (!node) return;
	const heldFocus = node.contains(document.activeElement);
	node.remove();
	const back = focusReturn.get(node);
	if (heldFocus && back?.isConnected) back.focus();
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
