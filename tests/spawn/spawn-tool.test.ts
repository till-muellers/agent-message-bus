import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { spawnToolDefinition } from "../../extension/lib/spawn/spawn-tool.ts";
import { type FakeNode, toJson, z } from "../fakes.ts";

const pi = { zod: z } as unknown as ExtensionAPI;
const tool = spawnToolDefinition(pi, async () => ({ name: "w", space: "demo", launcher: "tmux" }));

describe("bus_spawn_worker", () => {
	test("its description and argument texts are the documented ones (tool-descriptions.feature)", () => {
		expect(tool.description.startsWith("Starts a worker agent")).toBe(true);
		expect(tool.description.endsWith("Leaders of the space and admins.")).toBe(true);
		const properties = toJson(tool.parameters as unknown as FakeNode).properties ?? {};
		expect(properties.space?.description).toBe("Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.");
		expect(properties.task?.description).toBe("Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report.");
	});

	test("it is discoverable and needs exec approval", () => {
		expect([tool.loadMode, tool.approval]).toEqual(["discoverable", "exec"]);
	});
});
