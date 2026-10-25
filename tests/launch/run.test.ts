import { describe, expect, test } from "bun:test";
import { executeLaunch, LAUNCH_TIMEOUT_MS } from "../../extension/lib/launch/run.ts";
import type { RunOptions, RunResult } from "../../extension/lib/runner.ts";

function deps(result: RunResult) {
	const calls: { kind: string; command: string; args: string[]; options?: RunOptions }[] = [];
	return {
		calls,
		runner: async (command: string, args: string[], options?: RunOptions) => {
			calls.push({ kind: "run", command, args, options });
			return result;
		},
		spawnDetached: async (command: string, args: string[]) => {
			calls.push({ kind: "detached", command, args });
		},
	};
}

describe("executeLaunch", () => {
	test("runs non-detached commands through the runner with the launch timeout and the caller's signal", async () => {
		const d = deps({ code: 0, stdout: "", stderr: "" });
		const signal = new AbortController().signal;
		await executeLaunch({ command: "tmux", args: ["new-window"], detached: false }, d, signal);
		expect(d.calls).toEqual([{ kind: "run", command: "tmux", args: ["new-window"], options: { timeoutMs: LAUNCH_TIMEOUT_MS, signal } }]);
		expect(LAUNCH_TIMEOUT_MS).toBe(15_000);
	});
	test("spawns detached commands without the runner", async () => {
		const d = deps({ code: 0, stdout: "", stderr: "" });
		await executeLaunch({ command: "xterm", args: ["-e", "omp"], detached: true }, d);
		expect(d.calls).toEqual([{ kind: "detached", command: "xterm", args: ["-e", "omp"] }]);
	});
	test("a non-zero exit throws with stderr", async () => {
		const d = deps({ code: 2, stdout: "out", stderr: " bad tab\n" });
		await expect(executeLaunch({ command: "kitty", args: [], detached: false }, d)).rejects.toThrow("kitty exited with 2: bad tab");
	});
	test("a non-zero exit falls back to stdout", async () => {
		const d = deps({ code: 1, stdout: "only out\n", stderr: "" });
		await expect(executeLaunch({ command: "wt.exe", args: [], detached: false }, d)).rejects.toThrow("wt.exe exited with 1: only out");
	});
});
