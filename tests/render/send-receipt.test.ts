/** The text of a send result (`omp-tools.feature` "Send results read like omp's delivery receipts"). */
import { describe, expect, test } from "bun:test";
import { receiptText } from "../../extension/lib/render/send-receipt.ts";

const sent = (...receipts: { to: string; delivery: string }[]) => ({ ok: true, receipts });

describe("the send receipt text", () => {
	test.each([
		["direct", sent({ to: "peer", delivery: "live" }), "Delivered to peer."],
		["direct", sent({ to: "peer", delivery: "queued" }), "Queued for peer; it is not connected right now."],
		["topic", sent({ to: "lead", delivery: "queued" }, { to: "peer", delivery: "live" }), "Delivered to 2 subscriber(s): peer live, lead queued."],
		["topic", sent(), "Delivered to 0 subscriber(s)."],
	] as const)("%s with %j reads %p", (kind, result, text) => {
		expect(receiptText(kind, result)).toBe(text);
	});

	test("unknown fields of the result and of a receipt are ignored", () => {
		const result = { ok: true, extra: 1, receipts: [{ to: "peer", delivery: "live", since: "now" }] };
		expect(receiptText("direct", result)).toBe("Delivered to peer.");
	});

	test("a result without valid receipts has no receipt text", () => {
		expect(receiptText("direct", { ok: true })).toBeUndefined();
		expect(receiptText("direct", sent())).toBeUndefined();
		expect(receiptText("direct", sent({ to: "peer", delivery: "parked" }))).toBeUndefined();
		expect(receiptText("topic", null)).toBeUndefined();
	});
});
