import { describe, expect, test } from "bun:test";
import type { LaunchContext } from "../../extension/lib/launch/launcher.ts";
import { tmuxLauncher } from "../../extension/lib/launch/tmux.ts";

const ctx = (env: NodeJS.ProcessEnv): LaunchContext => ({
	env,
	platform: "linux",
	which: async () => undefined,
});

const spec = {
	title: "worker",
	argv: [
		"omp",
		"--cwd",
		"/w",
		"--bus-url",
		"http://127.0.0.1:8080",
		"--bus-accept",
		"k7m2-x9pq",
	],
} as const;

describe("tmuxLauncher", () => {
	test("id is tmux", () => {
		expect(tmuxLauncher.id).toBe("tmux");
	});

	test("detects a tmux session", async () => {
		expect(await tmuxLauncher.detect(ctx({ TMUX: "/tmp/tmux-1000/default,1234,0" }))).toBe(true);
	});

	test("does not detect outside tmux", async () => {
		expect(await tmuxLauncher.detect(ctx({}))).toBe(false);
		expect(await tmuxLauncher.detect(ctx({ TMUX: "" }))).toBe(false);
	});

	test("builds a new-window command with the exact argv", async () => {
		expect(await tmuxLauncher.command(spec, ctx({ TMUX: "1" }))).toEqual({
			command: "tmux",
			args: [
				"new-window",
				"-n",
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
