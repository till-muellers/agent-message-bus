import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { weztermLauncher } from "../../extension/lib/launch/wezterm.ts";

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

function ctxWith(env: NodeJS.ProcessEnv): LaunchContext {
	return { env, platform: "linux", which: async () => undefined };
}

describe("weztermLauncher", () => {
	test("id is wezterm", () => {
		expect(weztermLauncher.id).toBe("wezterm");
	});

	test("detects a wezterm pane", async () => {
		expect(await weztermLauncher.detect(ctxWith({ WEZTERM_PANE: "3" }))).toBe(true);
	});

	test("does not detect outside wezterm", async () => {
		expect(await weztermLauncher.detect(ctxWith({}))).toBe(false);
		expect(await weztermLauncher.detect(ctxWith({ WEZTERM_PANE: "" }))).toBe(false);
	});

	test("spawns the worker argv", async () => {
		const cmd = await weztermLauncher.command(spec, ctxWith({ WEZTERM_PANE: "3" }));
		expect(cmd).toEqual({
			command: "wezterm",
			args: [
				"cli",
				"spawn",
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
