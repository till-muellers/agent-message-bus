import { describe, expect, test } from "bun:test";
import { spawnRunner } from "../extension/lib/runner.ts";

/** A child process that would outlive every test. Real time on purpose: execFile's own timeout kills it, which fake timers cannot drive. */
const sleeper: [string, string[]] = [process.execPath, ["-e", "setTimeout(() => {}, 30000)"]];

describe("spawnRunner", () => {
	test("resolves with the exit code and output", async () => {
		expect(await spawnRunner(process.execPath, ["-e", "console.log('out'); process.exit(3)"])).toEqual({ code: 3, stdout: "out\n", stderr: "" });
	});

	test("kills a command that outlives its timeout and rejects", async () => {
		await expect(spawnRunner(...sleeper, { timeoutMs: 200 })).rejects.toThrow(`run ${process.execPath}: timed out after 200 ms`);
	});

	test("kills a command when its signal aborts and rejects", async () => {
		const controller = new AbortController();
		const run = spawnRunner(...sleeper, { signal: controller.signal });
		controller.abort();
		await expect(run).rejects.toThrow(`run ${process.execPath}: aborted`);
	});
});
