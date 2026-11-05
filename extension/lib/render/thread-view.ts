/**
 * The transcript view of a `bus_read_thread` result (`omp-blackboards.feature` rule 5): the thread title,
 * then each post with its author and time and its body quoted. A long thread shows only its newest posts,
 * and each body its first lines, until the result is expanded. A result that is not a thread (an error) is
 * shown as its text.
 */
import type { CardComponents, CardTheme } from "./entry-card.ts";
import { bodyLines, printable } from "./quote.ts";

/** A collapsed thread shows this many of its newest posts. */
const NEWEST_POSTS = 3;

type View = InstanceType<CardComponents["Container"]>;

interface Post {
	author: string;
	body: string;
	createdAt: string;
}

interface Thread {
	id: number;
	blackboard: string;
	title: string;
	posts: Post[];
}

function isPostRecord(value: unknown): value is Post {
	const post = value as Partial<Post> | null;
	return typeof post?.author === "string" && typeof post.body === "string" && typeof post.createdAt === "string";
}

/** The thread in the tool's JSON text; undefined when the text is not one. */
function parseThread(text: string): Thread | undefined {
	try {
		const thread = JSON.parse(text) as Partial<Thread> | null;
		const valid = typeof thread?.title === "string" && typeof thread.blackboard === "string" && typeof thread.id === "number";
		return valid && Array.isArray(thread.posts) && thread.posts.every(isPostRecord) ? (thread as Thread) : undefined;
	} catch {
		return undefined;
	}
}

function heading(thread: Thread, theme: CardTheme): string {
	const count = `${thread.posts.length} ${thread.posts.length === 1 ? "post" : "posts"}`;
	return `${theme.fg("accent", theme.bold(printable(thread.title)))} ${theme.fg("dim", `${thread.blackboard} / ${thread.id} · ${count}`)}`;
}

function postLines(post: Post, expanded: boolean, theme: CardTheme): string[] {
	return [`${theme.bold(printable(post.author))} ${theme.fg("dim", printable(post.createdAt))}`, ...bodyLines(post.body, expanded, theme)];
}

function threadLines(thread: Thread, expanded: boolean, theme: CardTheme): string[] {
	const shown = expanded ? thread.posts : thread.posts.slice(-NEWEST_POSTS);
	const older = thread.posts.length - shown.length;
	const hidden = older > 0 ? [theme.fg("dim", `… ${older} older ${older === 1 ? "post" : "posts"}`)] : [];
	return [heading(thread, theme), ...hidden, ...shown.flatMap(post => postLines(post, expanded, theme))];
}

/** The view of a `bus_read_thread` result. */
export function threadView(pi: CardComponents, result: { content: readonly { type: string; text?: string }[] }, expanded: boolean, theme: CardTheme): View {
	const text = result.content.map(part => (part.type === "text" ? (part.text ?? "") : "")).join("");
	const thread = parseThread(text);
	const view = new pi.Container();
	for (const line of thread ? threadLines(thread, expanded, theme) : [printable(text)]) view.addChild(new pi.Text(line, 0, 0));
	return view;
}
