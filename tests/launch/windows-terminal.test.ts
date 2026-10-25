import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { windowsTerminalLauncher } from "../../extension/lib/launch/windows-terminal.ts";

function ctxOf(platform: string, env: NodeJS.ProcessEnv): LaunchContext {
	return { env, platform, which: async () => undefined };
}

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

describe("windowsTerminalLauncher", () => {
	test("id is windows-terminal", () => {
		expect(windowsTerminalLauncher.id).toBe("windows-terminal");
	});

	test("detects a Windows Terminal session on win32", async () => {
		expect(await windowsTerminalLauncher.detect(ctxOf("win32", { WT_SESSION: "abc123" }))).toBe(true);
	});

	test("does not detect without WT_SESSION", async () => {
		expect(await windowsTerminalLauncher.detect(ctxOf("win32", {}))).toBe(false);
	});

	test("does not detect on other platforms", async () => {
		expect(await windowsTerminalLauncher.detect(ctxOf("linux", { WT_SESSION: "abc123" }))).toBe(false);
		expect(await windowsTerminalLauncher.detect(ctxOf("darwin", { WT_SESSION: "abc123" }))).toBe(false);
	});

	test("builds a new-tab command", async () => {
		const cmd = await windowsTerminalLauncher.command(spec, ctxOf("win32", { WT_SESSION: "abc123" }));
		expect(cmd).toEqual({
			command: "wt.exe",
			args: [
				"-w",
				"0",
				"new-tab",
				"--title",
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

	test("escapes semicolons in arguments", async () => {
		const cmd = await windowsTerminalLauncher.command(
			{ title: "worker", argv: ["omp", "--task", "a;b"] },
			ctxOf("win32", { WT_SESSION: "abc123" }),
		);
		expect(cmd.args).toEqual(["-w", "0", "new-tab", "--title", "worker", "--", "omp", "--task", "a\\;b"]);
	});
});
