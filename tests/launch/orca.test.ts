import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { orcaLauncher } from "../../extension/lib/launch/orca.ts";

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

function ctx(env: NodeJS.ProcessEnv, platform: string): LaunchContext {
	return { env, platform, which: async () => undefined };
}

describe("orcaLauncher", () => {
	test("detects an Orca terminal", async () => {
		expect(await orcaLauncher.detect(ctx({ TERM_PROGRAM: "Orca" }, "linux"))).toBe(true);
	});

	test("does not detect other environments", async () => {
		expect(await orcaLauncher.detect(ctx({}, "linux"))).toBe(false);
		expect(await orcaLauncher.detect(ctx({ TERM_PROGRAM: "iTerm.app" }, "darwin"))).toBe(false);
		expect(await orcaLauncher.detect(ctx({ TERM_PROGRAM: "Orca" }, "darwin"))).toBe(true);
	});

	test("builds a posix command on non-win32 platforms", async () => {
		const cmd = await orcaLauncher.command(spec, ctx({ TERM_PROGRAM: "Orca" }, "linux"));
		expect(cmd).toEqual({
			command: "orca",
			args: [
				"terminal",
				"create",
				"--title",
				"worker",
				"--command",
				"'omp' '--cwd' '/w' '--bus-url' 'http://127.0.0.1:8080' '--bus-accept' 'k7m2-x9pq'",
				"--json",
			],
			detached: false,
		});
	});

	test("builds a powershell command on win32", async () => {
		const cmd = await orcaLauncher.command(spec, ctx({ TERM_PROGRAM: "Orca" }, "win32"));
		expect(cmd).toEqual({
			command: "orca",
			args: [
				"terminal",
				"create",
				"--title",
				"worker",
				"--shell",
				"powershell.exe",
				"--command",
				"& 'omp' '--cwd' '/w' '--bus-url' 'http://127.0.0.1:8080' '--bus-accept' 'k7m2-x9pq'",
				"--json",
			],
			detached: false,
		});
	});

	test("uses ORCA_CLI_COMMAND as the executable", async () => {
		const cmd = await orcaLauncher.command(
			spec,
			ctx({ TERM_PROGRAM: "Orca", ORCA_CLI_COMMAND: "/opt/orca/bin/orca" }, "linux"),
		);
		expect(cmd.command).toBe("/opt/orca/bin/orca");
	});
});
