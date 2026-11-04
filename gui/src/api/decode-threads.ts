import { count, type Decoder, fields, flag, list, listOf, text } from "./decode.ts";
import type { Post, Thread, ThreadInfo } from "./types.ts";

// Decoders of the blackboard reads (contracts.md §10.2): bus_list_threads and bus_read_thread.

function threadInfo(json: unknown, what: string): ThreadInfo {
	const o = fields(json, what);
	return {
		id: count(o, "id", what),
		title: text(o, "title", what),
		createdBy: text(o, "createdBy", what),
		posts: count(o, "posts", what),
		lastPostAt: text(o, "lastPostAt", what),
	};
}

function post(json: unknown, what: string): Post {
	const o = fields(json, what);
	return { id: count(o, "id", what), author: text(o, "author", what), body: text(o, "body", what), createdAt: text(o, "createdAt", what) };
}

export const decodeThreadInfos: Decoder<ThreadInfo[]> = (json) => listOf(json, "threads", threadInfo);

export const decodeThread: Decoder<Thread> = (json) => {
	const o = fields(json, "thread");
	return {
		id: count(o, "id", "thread"),
		blackboard: text(o, "blackboard", "thread"),
		title: text(o, "title", "thread"),
		createdBy: text(o, "createdBy", "thread"),
		subscribed: flag(o, "subscribed", "thread"),
		posts: list(o, "posts", "thread", post),
	};
};
