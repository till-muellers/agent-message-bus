import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { spawnToolDefinition } from "../../extension/lib/spawn/spawn-tool.ts";
import { type FakeComponent, fakeComponents, type FakeNode, toJson, z } from "../fakes.ts";

const pi = { zod: z, pi: fakeComponents } as unknown as ExtensionAPI;
const tool = spawnToolDefinition(pi, async () => ({ name: "w", space: "demo", launcher: "tmux" }), () => "lead");

describe("bus_spawn_worker", () => {
	test("its description and argument texts are the documented ones (tool-descriptions.feature)", () => {
		expect(tool.description.startsWith("Starts a worker agent")).toBe(true);
		expect(tool.description.endsWith("Leaders of the space and admins.")).toBe(true);
		const properties = toJson(tool.parameters as unknown as FakeNode).properties ?? {};
		expect(properties.space?.description).toBe("Space name.");
		expect(properties.task?.description).toBe("Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report.");
	});

	test("it is discoverable and needs exec approval", () => {
		expect([tool.loadMode, tool.approval]).toEqual(["discoverable", "exec"]);
	});

	test("its call renders as an outgoing card: the task goes to the worker as a message from the caller", () => {
		const theme = { fg: (_color: string, text: string) => text, bold: (text: string) => text };
		const card = tool.renderCall?.({ space: "demo", name: "w1", task: "fix the build" }, { expanded: false, isPartial: false }, theme as never) as unknown as FakeComponent;
		expect(card.children.map(child => child.text)).toEqual(["lead → w1 · demo · spawn", "│ fix the build"]);
	});
});
