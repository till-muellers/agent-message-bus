import { describe, expect, test } from "bun:test";
import type { Runner } from "../../extension/lib/process/runner.ts";
import { createServerCli } from "../../extension/lib/local-server/server-cli.ts";

describe("server cli", () => {
	test("runs the resolved binary and returns stdout; a non-zero exit is an error with stderr", async () => {
		const runner: Runner = async (_command, args) => (args[0] === "ok" ? { code: 0, stdout: "out", stderr: "" } : { code: 1, stdout: "", stderr: "boom\n" });
		const cli = createServerCli(runner, async () => "/bin/amb");
		expect(await cli(["ok"])).toBe("out");
		await expect(cli(["stop"])).rejects.toThrow("agent-message-bus stop failed (exit 1): boom");
	});
});

