import type { Post, Thread } from "../api/types.ts";
import { hrefOf } from "../router/routes.ts";
import { h, type VNode } from "../view/h.ts";
import { type FormIntents, postForm } from "./forms.ts";
import { time } from "./time.ts";

function post(p: Post): VNode {
	return h("article", { key: String(p.id) }, h("header", null, h("strong", null, p.author), " ", time(p.createdAt)), h("p", null, p.body));
}

function origin(space: string, t: Thread): VNode {
	const board = h("a", { href: hrefOf({ name: "board", space, board: t.blackboard }) }, t.blackboard);
	return h("p", null, "on blackboard ", board, ` created by ${t.createdBy}`);
}

/** A thread: title, its blackboard and author, every post in order with author and time, then the reply form. */
export function threadView(space: string, t: Thread, intents: Pick<FormIntents, "post">): VNode {
	const key = `thread-${space}-${t.id}`;
	return h("section", { key }, h("h1", null, t.title), origin(space, t), t.posts.map(post), postForm(space, t.id, intents));
}
