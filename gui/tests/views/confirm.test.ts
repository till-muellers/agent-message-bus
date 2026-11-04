import { describe, expect, test } from "bun:test";
import type { PendingConfirm } from "../../src/state/app-state.ts";
import { confirmView } from "../../src/views/confirm.ts";
import { textOf, findAll } from "../tree.ts";

function buttons(pending: PendingConfirm, calls: string[]): Map<string, () => void> {
	const view = confirmView(pending, { confirmed: (p) => void calls.push(`yes ${p.kind}`), dismiss: () => void calls.push("no") });
	return new Map(findAll(view, (n) => n.tag === "button").map((b) => [textOf(b), () => b.on.click?.(new Event("click"))]));
}

describe("the confirmation", () => {
	test("a revoke names the space, the target and its role", () => {
		const view = confirmView({ kind: "revoke", space: "demo", name: "lead2", role: "leader" }, { confirmed: () => {}, dismiss: () => {} });
		expect(textOf(view)).toContain("Revoke leader lead2 from space demo?");
	});

	test("a delete names the space", () => {
		const view = confirmView({ kind: "delete-space", space: "demo" }, { confirmed: () => {}, dismiss: () => {} });
		expect(textOf(view)).toContain("Delete space demo");
	});

	test("confirming runs it, cancelling dismisses it", () => {
		const calls: string[] = [];
		const map = buttons({ kind: "delete-space", space: "demo" }, calls);
		map.get("Delete space")?.();
		map.get("Cancel")?.();
		expect(calls).toEqual(["yes delete-space", "no"]);
	});
});
