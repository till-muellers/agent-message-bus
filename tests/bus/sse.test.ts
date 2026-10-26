import { describe, expect, test } from "bun:test";
import { parseSse } from "../../extension/lib/bus/sse.ts";
import { streamOf } from "../fakes.ts";

async function collect<T>(it: AsyncIterable<T>): Promise<T[]> {
	const out: T[] = [];
	for await (const item of it) out.push(item);
	return out;
}

describe("parseSse", () => {
	test("splits frames, joins multi-line data, skips comments, handles CRLF and split chunks", async () => {
		const frames = await collect(
			parseSse(streamOf([": ping\n\n", "id: 1\r\nevent: inbox\r\ndata: a\r\n", "data: b\r\n\r\n", "id: 2\nev", "ent: inbox\ndata: c\n\n", "data: d\n\n"])),
		);
		expect(frames).toEqual([
			{ id: "1", event: "inbox", data: "a\nb" },
			{ id: "2", event: "inbox", data: "c" },
			{ event: "message", data: "d" },
		]);
	});
});
