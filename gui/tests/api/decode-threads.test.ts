import { describe, expect, test } from "bun:test";
import { decodeThread, decodeThreadInfos } from "../../src/api/decode-threads.ts";
import { thread, threadInfos } from "../fixtures.ts";

function invalid(): unknown {
	return expect.objectContaining({ code: "invalid_response" });
}

describe("decoding the threads of a blackboard", () => {
	test("the threads in the server's order", () => {
		expect(decodeThreadInfos(threadInfos)).toEqual(threadInfos);
	});

	test("a thread without its latest post time is refused", () => {
		expect(() => decodeThreadInfos([{ id: 1, title: "plan", createdBy: "w", posts: 1 }])).toThrow(invalid());
	});

	test("a body that is not a list is refused", () => {
		expect(() => decodeThreadInfos({})).toThrow(invalid());
	});
});

describe("decoding a thread", () => {
	test("the thread with its posts in order", () => {
		expect(decodeThread(thread)).toEqual(thread);
	});

	test("a post without an author is refused", () => {
		const wire = { ...thread, posts: [{ id: 1, body: "first", createdAt: "x" }] };
		expect(() => decodeThread(wire)).toThrow(invalid());
	});
});
