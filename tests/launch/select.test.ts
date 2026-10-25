import { describe, expect, test } from "bun:test";
import type { LaunchContext, Launcher } from "../../extension/lib/launch/launcher.ts";
import { pickLauncher } from "../../extension/lib/launch/select.ts";

const ctx: LaunchContext = { env: {}, platform: "linux", which: async () => undefined };

function fake(id: string, detected: boolean): Launcher {
	return {
		id,
		detect: async () => detected,
		command: async () => ({ command: id, args: [], detached: false }),
	};
}

describe("pickLauncher", () => {
	test("returns the first detected launcher in order", async () => {
		const picked = await pickLauncher([fake("a", false), fake("b", true), fake("c", true)], ctx);
		expect(picked?.id).toBe("b");
	});
	test("returns undefined when none is detected", async () => {
		expect(await pickLauncher([fake("a", false)], ctx)).toBeUndefined();
	});
});
