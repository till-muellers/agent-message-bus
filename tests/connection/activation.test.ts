import { describe, expect, test } from "bun:test";
import { activateBusTools, deactivateBusTools, deactivateTools } from "../../extension/lib/connection/activation.ts";

describe("activation", () => {
	test("deactivate drops every bus_* tool; activate re-adds the missing names only", async () => {
		let active = ["read", "bus_a", "edit", "bus_b"];
		const pi = { getActiveTools: () => [...active], setActiveTools: async (names: string[]) => void (active = names) };
		await deactivateBusTools(pi);
		expect(active).toEqual(["read", "edit"]);
		await activateBusTools(pi, ["bus_a", "bus_b"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
		await activateBusTools(pi, ["bus_a"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
	});

	test("deactivateTools drops exactly the given names", async () => {
		let active = ["read", "bus_a", "bus_foreign", "bus_b"];
		const pi = { getActiveTools: () => [...active], setActiveTools: async (names: string[]) => void (active = names) };
		await deactivateTools(pi, ["bus_a", "bus_b", "bus_missing"]);
		expect(active).toEqual(["read", "bus_foreign"]);
	});
});

