import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { macosTerminalLauncher } from "../../extension/lib/launch/macos-terminal.ts";

function ctx(platform: string): LaunchContext {
	return { env: {}, platform, which: async () => undefined };
}

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

describe("macosTerminalLauncher", () => {
	test("detects darwin", async () => {
		expect(await macosTerminalLauncher.detect(ctx("darwin"))).toBe(true);
	});

	test("does not detect other platforms", async () => {
		expect(await macosTerminalLauncher.detect(ctx("linux"))).toBe(false);
		expect(await macosTerminalLauncher.detect(ctx("win32"))).toBe(false);
	});

	test("builds an osascript command that opens a Terminal window", async () => {
		expect(await macosTerminalLauncher.command(spec, ctx("darwin"))).toEqual({
			command: "osascript",
			args: [
				"-e",
				'tell application "Terminal" to do script "\'omp\' \'--cwd\' \'/w\' \'--bus-url\' \'http://127.0.0.1:8080\' \'--bus-accept\' \'k7m2-x9pq\'"',
				"-e",
				'tell application "Terminal" to activate',
			],
			detached: false,
		});
	});

	test("escapes backslashes and quotes for AppleScript", async () => {
		const quoted: WorkerSpec = { title: "worker", argv: ["omp", "--cwd", "/a\\b", "--note", 'say "hi"'] };
		const cmd = await macosTerminalLauncher.command(quoted, ctx("darwin"));
		expect(cmd.args[1]).toContain('\\"hi\\"');
		expect(cmd.args[1]).toContain("\\\\b");
	});
});
