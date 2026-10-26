import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createDeliver, deliveryOptions } from "../../extension/lib/delivery/delivery-class.ts";
import { formatEntry } from "../../extension/lib/render/envelope.ts";
import { entry } from "../fakes.ts";

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
