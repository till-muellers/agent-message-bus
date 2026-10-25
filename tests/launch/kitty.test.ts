import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { kittyLauncher } from "../../extension/lib/launch/kitty.ts";

function ctx(env: NodeJS.ProcessEnv): LaunchContext {
	return { env, platform: "linux", which: async () => undefined };
}

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

describe("kittyLauncher", () => {
	test("id is kitty", () => {
		expect(kittyLauncher.id).toBe("kitty");
	});

	test("detects kitty from KITTY_WINDOW_ID", async () => {
		expect(await kittyLauncher.detect(ctx({ KITTY_WINDOW_ID: "3" }))).toBe(true);
	});

	test("does not detect kitty without KITTY_WINDOW_ID", async () => {
		expect(await kittyLauncher.detect(ctx({}))).toBe(false);
		expect(await kittyLauncher.detect(ctx({ KITTY_WINDOW_ID: "" }))).toBe(false);
	});

	test("builds the launch command", async () => {
		expect(await kittyLauncher.command(spec, ctx({}))).toEqual({
			command: "kitty",
			args: [
				"@",
				"launch",
				"--type=tab",
				"--tab-title",
				"worker",
				"--",
				"omp",
				"--cwd",
				"/w",
				"--bus-url",
				"http://127.0.0.1:8080",
				"--bus-accept",
				"k7m2-x9pq",
			],
			detached: false,
		});
	});
});
