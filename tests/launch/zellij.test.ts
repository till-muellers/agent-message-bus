import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { zellijLauncher } from "../../extension/lib/launch/zellij.ts";

function makeCtx(env: NodeJS.ProcessEnv): LaunchContext {
	return { env, platform: "linux", which: async () => undefined };
}

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

describe("zellijLauncher", () => {
	test("detects zellij via ZELLIJ", async () => {
		expect(await zellijLauncher.detect(makeCtx({ ZELLIJ: "0" }))).toBe(true);
	});
	test("does not detect zellij without ZELLIJ", async () => {
		expect(await zellijLauncher.detect(makeCtx({}))).toBe(false);
	});
	test("opens a new tab running the worker argv", async () => {
		expect(await zellijLauncher.command(spec, makeCtx({ ZELLIJ: "0" }))).toEqual({
			command: "zellij",
			args: [
				"action",
				"new-tab",
				"--name",
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
