/**
 * The contract every terminal launcher must satisfy (findings item 7, Tier 0).
 *
 * The per-launcher tests assert the argv of one launcher each; this file asserts
 * the properties every launcher shares: it detects itself only in its own
 * environment, it names a command, it says whether the command detaches, and it
 * hands the worker's argv over without letting a shell re-read it. It also
 * pins the property that keeps a failed spawn from burning an invite code: with
 * no launcher detected, nobody is invited.
 */
import { describe, expect, test } from "bun:test";
import type { BusClient } from "../../extension/lib/bus/client.ts";
import { LAUNCHERS } from "../../extension/lib/launch/index.ts";
import type { LaunchContext, Launcher, WorkerSpec } from "../../extension/lib/launch/launcher.ts";
import { pickLauncher } from "../../extension/lib/launch/select.ts";
import { type SpawnDeps, spawnWorker } from "../../extension/lib/spawn/spawn.ts";

const SPEC: WorkerSpec = {
	title: "worker",
	argv: ["omp", "--cwd", "/w dir", "--bus-url", "http://127.0.0.1:8080", "--bus-accept", "k7m2-x9pq"],
};

/** How a launcher hands the worker's argv to the terminal. */
type ArgvForm =
	/** One argument per argv element: the terminal takes them as separate arguments. */
	| "arguments"
	/** The whole argv as one quoted shell line. */
	| "quoted-line"
	/** One quoted argument per argv element, inside a script. */
	| "quoted-items";

interface Contract {
	/** The launcher's id. */
	id: string;
	launcher: Launcher;
	/** The environment and platform in which the launcher detects itself. */
	detected: { env: NodeJS.ProcessEnv; platform: string; onPath?: readonly string[] };
	/** The environment and platform in which it must not detect itself. */
	undetected: { env: NodeJS.ProcessEnv; platform: string; onPath?: readonly string[] };
	/** Whether the launcher's command detaches: it may block until the window closes. */
	detached: boolean;
	/** How the argv reaches the terminal. */
	argv: ArgvForm;
}

const CONTRACTS: readonly Contract[] = [
	{
		id: "tmux",
		launcher: LAUNCHERS[0],
		detected: { env: { TMUX: "/tmp/tmux-1000/default" }, platform: "linux" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "arguments",
	},
	{
		id: "zellij",
		launcher: LAUNCHERS[1],
		detected: { env: { ZELLIJ: "session" }, platform: "linux" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "arguments",
	},
	{
		id: "orca",
		launcher: LAUNCHERS[2],
		detected: { env: { TERM_PROGRAM: "Orca" }, platform: "linux" },
		undetected: { env: { TERM_PROGRAM: "iTerm.app" }, platform: "linux" },
		detached: false,
		argv: "quoted-line",
	},
	{
		id: "wezterm",
		launcher: LAUNCHERS[3],
		detected: { env: { WEZTERM_PANE: "3" }, platform: "linux" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "arguments",
	},
	{
		id: "kitty",
		launcher: LAUNCHERS[4],
		detected: { env: { KITTY_WINDOW_ID: "1" }, platform: "linux" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "arguments",
	},
	{
		id: "windows-terminal",
		launcher: LAUNCHERS[5],
		detected: { env: { WT_SESSION: "a-guid" }, platform: "win32" },
		undetected: { env: { WT_SESSION: "a-guid" }, platform: "linux" },
		detached: false,
		argv: "arguments",
	},
	{
		id: "macos-terminal",
		launcher: LAUNCHERS[6],
		detected: { env: {}, platform: "darwin" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "quoted-line",
	},
	{
		id: "linux-terminal",
		launcher: LAUNCHERS[7],
		detected: { env: { DISPLAY: ":0" }, platform: "linux", onPath: ["xterm"] },
		undetected: { env: { DISPLAY: ":0" }, platform: "linux" },
		detached: true,
		argv: "arguments",
	},
	{
		id: "windows-console",
		launcher: LAUNCHERS[8],
		detected: { env: {}, platform: "win32" },
		undetected: { env: {}, platform: "linux" },
		detached: false,
		argv: "quoted-items",
	},
];

function context(where: { env: NodeJS.ProcessEnv; platform: string; onPath?: readonly string[] }): LaunchContext {
	const onPath = where.onPath ?? [];
	return { env: where.env, platform: where.platform, which: async name => (onPath.includes(name) ? `/usr/bin/${name}` : undefined) };
}

/** The clients of the spawn contract: a client that records every call. */
function recordingClient(calls: unknown[][]): BusClient {
	return {
		listTools: async () => [],
		openStream: async () => new Response(),
		acceptInvite: async () => ({ name: "", space: "", token: "" }),
		async callTool(url, token, name, args) {
			calls.push([url, token, name, args]);
			return { name: args.name, space: args.space, code: "k7m2-x9pq", expires_at: "2026-10-25T00:10:00Z" };
		},
	};
}

describe("launcher contract", () => {
	test("all 9 launchers are covered", () => {
		expect(CONTRACTS.map(c => c.id)).toEqual([
			"tmux",
			"zellij",
			"orca",
			"wezterm",
			"kitty",
			"windows-terminal",
			"macos-terminal",
			"linux-terminal",
			"windows-console",
		]);
		expect(CONTRACTS.map(c => c.launcher.id)).toEqual(CONTRACTS.map(c => c.id));
		expect(CONTRACTS.map(c => c.launcher)).toEqual([...LAUNCHERS]);
	});

	for (const contract of CONTRACTS) {
		test(`${contract.id}: detects itself only in its own environment`, async () => {
			expect(await contract.launcher.detect(context(contract.detected))).toBe(true);
			expect(await contract.launcher.detect(context(contract.undetected))).toBe(false);
		});

		test(`${contract.id}: names a command and says whether it detaches`, async () => {
			const cmd = await contract.launcher.command(SPEC, context(contract.detected));
			expect(cmd.command).not.toBe("");
			expect(cmd.detached).toBe(contract.detached);
		});

		test(`${contract.id}: hands the worker's argv over without letting a shell re-read it`, async () => {
			const cmd = await contract.launcher.command(SPEC, context(contract.detected));
			const joined = cmd.args.join(" ");
			if (contract.argv === "arguments") {
				for (const arg of SPEC.argv) expect(cmd.args).toContain(arg);
				// A shell must never see the argv as one line: the path with a space stays one argument.
				expect(cmd.args).toContain("/w dir");
			} else if (contract.argv === "quoted-line") {
				for (const arg of SPEC.argv) expect(joined).toContain(`'${arg}'`);
				expect(joined).not.toContain(" /w dir ");
			} else {
				for (const arg of SPEC.argv.slice(1)) expect(joined).toContain(`"${arg}"`);
				expect(joined).not.toContain(" /w dir ");
			}
		});
	}

	test("no launcher detected means nobody is invited", async () => {
		const calls: unknown[][] = [];
		const nowhere = context({ env: {}, platform: "linux" });
		expect(await pickLauncher(LAUNCHERS, nowhere)).toBeUndefined();
		const deps: SpawnDeps = {
			client: recordingClient(calls),
			getActive: () => ({ url: "http://127.0.0.1:8080", token: "lead-tok" }),
			launchers: LAUNCHERS,
			launchContext: nowhere,
			pluginArgs: [],
			ownVersion: "0.2.0",
			workerVersion: async () => undefined,
			execute: async () => {},
		};
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toThrow("no terminal launcher found for this environment");
		expect(calls).toEqual([]);
	});
});
