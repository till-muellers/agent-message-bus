import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createDeliver, deliveryOptions } from "../extension/lib/delivery/delivery-class.ts";
import { formatEntry } from "../extension/lib/render/envelope.ts";
import { parseSse } from "../extension/lib/bus/sse.ts";
import { entry, streamOf } from "./fakes.ts";

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

describe("formatEntry", () => {
	test("renders each type", () => {
		expect(formatEntry(entry(1, { critical: true }))).toBe("[agent-bus] demo · direct · from lead · CRITICAL\nbody 1");
		expect(formatEntry(entry(2, { type: "topic_message", topic: "all" }))).toStartWith("[agent-bus] demo · topic #all · from lead\n");
		expect(formatEntry(entry(3, { type: "topic_closed", topic: "x" }))).toContain("· topic #x closed ·");
		expect(formatEntry(entry(4, { type: "thread_post", threadId: 7, threadTitle: "T", blackboard: "bb" }))).toContain('· thread 7 "T" on bb ·');
		expect(formatEntry(entry(5, { type: "thread_removed", threadId: 7, threadTitle: "T" }))).toContain('· thread 7 "T" removed ·');
		expect(formatEntry(entry(6, { type: "blackboard_removed", blackboard: "bb" }))).toContain("· blackboard bb removed ·");
	});
});

describe("deliver", () => {
	test("deliveryOptions: critical steers, others follow up; only tui and rpc trigger a turn", () => {
		for (const [critical, mode, expected] of [
			[true, "tui", { deliverAs: "steer", triggerTurn: true }],
			[false, "rpc", { deliverAs: "followUp", triggerTurn: true }],
			[true, "print", { deliverAs: "steer", triggerTurn: false }],
			[false, "json", { deliverAs: "followUp", triggerTurn: false }],
			[false, undefined, { deliverAs: "followUp", triggerTurn: false }],
		] as const) {
			expect({ critical, mode, options: deliveryOptions(entry(1, { critical }), mode) }).toEqual({ critical, mode, options: expected });
		}
	});

	test("createDeliver sends the formatted entry with the options for the mode at delivery time", async () => {
		const sent: unknown[] = [];
		let mode = "print";
		const pi = { sendMessage: (message: unknown, options: unknown) => void sent.push({ message, options }) } as unknown as ExtensionAPI;
		const deliver = createDeliver(pi, () => mode);
		await deliver(entry(1));
		mode = "tui";
		await deliver(entry(2, { critical: true }));
		expect(sent).toEqual([
			{ message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true }, options: { deliverAs: "followUp", triggerTurn: false } },
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { critical: true })), display: true },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
	});
});
