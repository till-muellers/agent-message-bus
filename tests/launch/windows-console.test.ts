import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { windowsConsoleLauncher } from "../../extension/lib/launch/windows-console.ts";

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

function ctxFor(platform: string): LaunchContext {
	return { env: {}, platform, which: async () => undefined };
}

describe("windowsConsoleLauncher", () => {
	test("detects windows", async () => {
		expect(await windowsConsoleLauncher.detect(ctxFor("win32"))).toBe(true);
	});

	test("does not detect other platforms", async () => {
		expect(await windowsConsoleLauncher.detect(ctxFor("linux"))).toBe(false);
		expect(await windowsConsoleLauncher.detect(ctxFor("darwin"))).toBe(false);
	});

	test("builds the powershell Start-Process command", async () => {
		const cmd = await windowsConsoleLauncher.command(spec, ctxFor("win32"));
		expect(cmd).toEqual({
			command: "powershell.exe",
			args: [
				"-NoProfile",
				"-NonInteractive",
				"-Command",
				`Start-Process -FilePath 'omp' -ArgumentList '"--cwd"','"/w"','"--bus-url"','"http://127.0.0.1:8080"','"--bus-accept"','"k7m2-x9pq"'`,
			],
			detached: false,
		});
	});

	test("keeps paths with spaces intact", async () => {
		const cmd = await windowsConsoleLauncher.command(
			{ title: "worker", argv: ["omp", "--cwd", "C:\\Program Files\\w"] },
			ctxFor("win32"),
		);
		expect(cmd.args[3]).toBe(
			`Start-Process -FilePath 'omp' -ArgumentList '"--cwd"','"C:\\Program Files\\w"'`,
		);
	});

	test("doubles trailing backslashes so the closing quote stays a quote", async () => {
		const cmd = await windowsConsoleLauncher.command(
			{ title: "worker", argv: ["omp", "--cwd", "C:\\"] },
			ctxFor("win32"),
		);
		expect(cmd.args[3]).toBe(`Start-Process -FilePath 'omp' -ArgumentList '"--cwd"','"C:\\\\"'`);
	});

	test("doubles trailing backslashes of a path with spaces", async () => {
		const cmd = await windowsConsoleLauncher.command(
			{ title: "worker", argv: ["omp", "--cwd", "C:\\My Work\\"] },
			ctxFor("win32"),
		);
		expect(cmd.args[3]).toBe(`Start-Process -FilePath 'omp' -ArgumentList '"--cwd"','"C:\\My Work\\\\"'`);
	});
});
