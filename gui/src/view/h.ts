export type Handler = (event: Event) => void;
export type Handlers = Readonly<Record<string, Handler>>;
export type AttrValue = string | number | boolean | null | undefined;

export interface Props {
	readonly key?: string;
	readonly on?: Handlers;
	readonly [attr: string]: AttrValue | Handlers;
}

export interface VNode {
	readonly tag: string;
	readonly key: string | undefined;
	readonly attrs: Readonly<Record<string, string>>;
	readonly on: Handlers;
	readonly children: readonly VChild[];
}

export type VChild = VNode | string;
export type ChildArg = VChild | number | boolean | null | undefined | readonly ChildArg[];

function attrsOf(props: Props): Record<string, string> {
	const attrs: Record<string, string> = {};
	for (const [name, value] of Object.entries(props)) {
		if (name === "key" || name === "on" || typeof value === "object" || value === undefined || value === false) continue;
		attrs[name] = value === true ? "" : String(value);
	}
	return attrs;
}

function isList(child: ChildArg): child is readonly ChildArg[] {
	return Array.isArray(child);
}

function flatten(children: readonly ChildArg[]): VChild[] {
	return children.flatMap((child): VChild[] => {
		if (isList(child)) return flatten(child);
		if (child === null || child === undefined || typeof child === "boolean") return [];
		return [typeof child === "number" ? String(child) : child];
	});
}

/** Builds a virtual node; texts become text nodes, never HTML. */
export function h(tag: string, props: Props | null, ...children: ChildArg[]): VNode {
	return { tag, key: props?.key, attrs: props ? attrsOf(props) : {}, on: props?.on ?? {}, children: flatten(children) };
}
