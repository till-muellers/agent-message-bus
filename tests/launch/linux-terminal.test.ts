import { describe, expect, test } from "bun:test";
import type { LaunchContext, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { linuxTerminalLauncher } from "../../extension/lib/launch/linux-terminal.ts";

const spec: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

function makeCtx(platform: string, found: readonly string[] = [], env: NodeJS.ProcessEnv = { DISPLAY: ":0" }): LaunchContext {
	return {
		env,
		platform,
		which: async (name: string) => (found.includes(name) ? `/usr/bin/${name}` : undefined),
	};
}

describe("linuxTerminalLauncher", () => {
	test("detects only on linux with one of the terminals on PATH", async () => {
		expect(await linuxTerminalLauncher.detect(makeCtx("linux", ["xterm"]))).toBe(true);
		expect(await linuxTerminalLauncher.detect(makeCtx("darwin", ["xterm"]))).toBe(false);
		expect(await linuxTerminalLauncher.detect(makeCtx("win32", ["xterm"]))).toBe(false);
	});

	test("does not detect when no terminal emulator is on PATH, so nothing gets invited", async () => {
		expect(await linuxTerminalLauncher.detect(makeCtx("linux", ["alacritty"]))).toBe(false);
	});

	test("detects only with a graphical display", async () => {
		expect(await linuxTerminalLauncher.detect(makeCtx("linux", ["xterm"], {}))).toBe(false);
		expect(await linuxTerminalLauncher.detect(makeCtx("linux", ["xterm"], { WAYLAND_DISPLAY: "wayland-0" }))).toBe(true);
	});

	test("uses x-terminal-emulator when it is found", async () => {
		expect(await linuxTerminalLauncher.command(spec, makeCtx("linux", ["x-terminal-emulator"]))).toEqual({
			command: "x-terminal-emulator",
			args: ["-e", ...spec.argv],
			detached: true,
		});
	});

	test("uses gnome-terminal when it is the first found", async () => {
		expect(await linuxTerminalLauncher.command(spec, makeCtx("linux", ["gnome-terminal", "xterm"]))).toEqual({
			command: "gnome-terminal",
			args: ["--", ...spec.argv],
			detached: true,
		});
	});

	test("uses konsole when it is the first found", async () => {
		expect(await linuxTerminalLauncher.command(spec, makeCtx("linux", ["konsole", "xterm"]))).toEqual({
			command: "konsole",
			args: ["-e", ...spec.argv],
			detached: true,
		});
	});

	test("uses xterm when it is the only one found", async () => {
		expect(await linuxTerminalLauncher.command(spec, makeCtx("linux", ["xterm"]))).toEqual({
			command: "xterm",
			args: ["-e", ...spec.argv],
			detached: true,
		});
	});

	test("command never fails on the environment: without a terminal on PATH it still names x-terminal-emulator", async () => {
		expect(await linuxTerminalLauncher.command(spec, makeCtx("linux"))).toEqual({
			command: "x-terminal-emulator",
			args: ["-e", ...spec.argv],
			detached: true,
		});
	});
});
