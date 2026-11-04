/** The text of a send result (`omp-tools.feature` "Send results read like omp's delivery receipts"). */
import { describe, expect, test } from "bun:test";
import { receiptText } from "../../extension/lib/render/send-receipt.ts";

const sent = (...receipts: { to: string; delivery: string }[]) => ({ ok: true, receipts });

describe("the send receipt text", () => {
	test.each([
		["bus_send_direct", sent({ to: "peer", delivery: "live" }), "Delivered to peer."],
		["bus_send_direct", sent({ to: "peer", delivery: "queued" }), "Queued for peer; it is not connected right now."],
		["bus_send_topic", sent({ to: "lead", delivery: "queued" }, { to: "peer", delivery: "live" }), "Delivered to 2 subscriber(s): peer live, lead queued."],
		["bus_send_topic", sent(), "Delivered to 0 subscriber(s)."],
	])("%s with %j reads %p", (tool, result, text) => {
		expect(receiptText(tool, result)).toBe(text);
	});

	test("unknown fields of the result and of a receipt are ignored", () => {
		const result = { ok: true, extra: 1, receipts: [{ to: "peer", delivery: "live", since: "now" }] };
		expect(receiptText("bus_send_direct", result)).toBe("Delivered to peer.");
	});

	test("a result without valid receipts, or of another tool, has no receipt text", () => {
		expect(receiptText("bus_send_direct", { ok: true })).toBeUndefined();
		expect(receiptText("bus_send_direct", sent())).toBeUndefined();
		expect(receiptText("bus_send_direct", sent({ to: "peer", delivery: "parked" }))).toBeUndefined();
		expect(receiptText("bus_send_topic", null)).toBeUndefined();
		expect(receiptText("bus_add_post", { postId: 7, ...sent({ to: "peer", delivery: "live" }) })).toBeUndefined();
	});
});
