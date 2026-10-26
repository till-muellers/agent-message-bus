import { describe, expect, test } from "bun:test";
import { type BusClient, BusError } from "../../extension/lib/bus/client.ts";
import { LAUNCHERS } from "../../extension/lib/launch/index.ts";
import type { LaunchCommand, LaunchContext, Launcher } from "../../extension/lib/launch/launcher.ts";
import { type SpawnDeps, spawnWorker } from "../../extension/lib/spawn/spawn.ts";
import GOLDEN_ARGV from "./spawn.argv.golden.json";

const URL_ = "http://127.0.0.1:8080";
const CODE = "k7m2-x9pq";
const launchContext: LaunchContext = { env: {}, platform: "linux", which: async () => undefined };

/** A launcher that is detected or not and builds `<id> ...argv`. */
function fakeLauncher(id: string, detected: boolean): Launcher {
	return {
		id,
		detect: async () => detected,
		command: async spec => ({ command: id, args: [spec.title, ...spec.argv], detached: false }),
	};
}

/** A client whose `bus_invite_member` answers `result` (default: the contract's `{name, space, code, expires_at}`) and records each call. */
function inviteClient(calls: unknown[][], result?: unknown): BusClient {
	return {
		listTools: async () => [],
		openStream: async () => new Response(),
		acceptInvite: async () => ({ name: "", space: "", token: "" }),
		async callTool(url, token, name, args) {
			calls.push([url, token, name, args]);
			return result ?? { name: args.name, space: args.space, code: CODE, expires_at: "2026-10-25T00:10:00Z" };
		},
	};
}

function setup(overrides: Partial<SpawnDeps> = {}) {
	const calls: unknown[][] = [];
	const executed: { cmd: LaunchCommand; signal?: AbortSignal }[] = [];
	const deps: SpawnDeps = {
		client: inviteClient(calls),
		getActive: () => ({ url: URL_, token: "lead-tok" }),
		launchers: [fakeLauncher("off", false), fakeLauncher("on", true), fakeLauncher("later", true)],
		launchContext,
		pluginArgs: [],
		ownVersion: "0.2.0",
		workerVersion: async () => undefined,
		execute: async (cmd, signal) => void executed.push({ cmd, signal }),
		...overrides,
	};
	return { deps, calls, executed };
}

const workerArgv = (...extra: string[]) => ["worker", "omp", ...extra, "--cwd", "/w", "--bus-url", URL_, "--bus-accept", CODE];

describe("spawnWorker", () => {
	test("invites the member and runs the first detected launcher with the exact worker argv and the caller's signal", async () => {
		const { deps, calls, executed } = setup();
		const signal = new AbortController().signal;
		const result = await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" }, signal);
		expect(result).toEqual({ name: "worker", space: "demo", launcher: "on" });
		expect(calls).toEqual([[URL_, "lead-tok", "bus_invite_member", { space: "demo", name: "worker" }]]);
		expect(executed).toEqual([{ cmd: { command: "on", args: workerArgv(), detached: false }, signal }]);
	});

	test("the task travels with the invite; an empty task is not sent", async () => {
		const { deps, calls } = setup();
		await spawnWorker(deps, { space: "demo", name: "worker", task: "fix the bug", cwd: "/w" });
		await spawnWorker(deps, { space: "demo", name: "worker", task: "", cwd: "/w" });
		expect(calls.map(c => c[3])).toEqual([
			{ space: "demo", name: "worker", task: "fix the bug" },
			{ space: "demo", name: "worker" },
		]);
	});

	test("forwards the leader's extension-loading flags so the worker knows --bus-url and --bus-accept", async () => {
		const { deps, executed } = setup({ pluginArgs: ["--plugin-dir", "D:/amb", "-e", "x.ts"] });
		await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" });
		expect(executed[0].cmd.args).toEqual(workerArgv("--plugin-dir", "D:/amb", "-e", "x.ts"));
	});

	test("throws without active credentials and invites nobody", async () => {
		const { deps, calls } = setup({ getActive: () => undefined });
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toThrow(
			"Not logged in to the agent bus. Run /bus login <url> <token>.",
		);
		expect(calls).toEqual([]);
	});

	test("without a detected launcher it errors before inviting, so no code is wasted", async () => {
		const { deps, calls } = setup({ launchers: [fakeLauncher("off", false)] });
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toThrow("no terminal launcher found for this environment");
		expect(calls).toEqual([]);
	});

	test("a worker with an older plugin is refused before the invite, so no code is wasted and no terminal opens", async () => {
		const { deps, calls, executed } = setup({ ownVersion: "0.2.0", workerVersion: async () => "0.1.0" });
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toThrow(
			"the worker would load omp plugin 0.1.0, which is older than the protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus",
		);
		expect(calls).toEqual([]);
		expect(executed).toEqual([]);
	});

	test("a worker with the same or a newer plugin version is spawned", async () => {
		for (const version of ["0.2.0", "0.3.0"]) {
			const { deps, calls } = setup({ ownVersion: "0.2.0", workerVersion: async () => version });
			expect(await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).toEqual({
				name: "worker",
				space: "demo",
				launcher: "on",
			});
			expect(calls).toEqual([[URL_, "lead-tok", "bus_invite_member", { space: "demo", name: "worker" }]]);
		}
	});

	test("an invite result without a code is an invalid response and launches nothing", async () => {
		const { deps, executed } = setup({ client: inviteClient([], { name: "worker", space: "demo", token: "amb_x" }) });
		const error = await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" }).catch(e => e);
		expect(error).toBeInstanceOf(BusError);
		expect(error).toMatchObject({ code: "invalid_response", message: "bus_invite_member: response has no code" });
		expect(executed).toEqual([]);
	});

	test("a launch failure is prefixed with the launcher id", async () => {
		const { deps } = setup({
			execute: async () => {
				throw new Error("tmux exited with 1: no server running");
			},
		});
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toThrow("on: tmux exited with 1: no server running");
	});

	test("an invite error propagates unchanged and launches nothing", async () => {
		const failing: BusClient = {
			...inviteClient([]),
			callTool: async () => {
				throw new BusError("forbidden", "only leaders of space demo can invite members");
			},
		};
		const { deps, executed } = setup({ client: failing });
		await expect(spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" })).rejects.toMatchObject({ code: "forbidden" });
		expect(executed).toEqual([]);
	});
});

describe("LAUNCHERS", () => {
	test("multiplexers first, then terminals, windows-console last", () => {
		expect(LAUNCHERS.map(l => l.id)).toEqual([
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
	});
});

describe("the worker argv", () => {
	/** A launcher that records the argv spawnWorker handed it, so the golden file holds the argv spawnWorker composed. */
	function recordingSetup(): { deps: SpawnDeps; argv: string[][] } {
		const argv: string[][] = [];
		const launcher: Launcher = {
			id: "on",
			detect: async () => true,
			command: async spec => {
				argv.push([...spec.argv]);
				return { command: "on", args: [...spec.argv], detached: false };
			},
		};
		const { deps } = setup({ launchers: [launcher], pluginArgs: ["--plugin-dir", "D:/amb", "-e", "x.ts"] });
		return { deps, argv };
	}

	test("is exactly the recorded argv: omp, the leader's flags, then --cwd, --bus-url and --bus-accept", async () => {
		const { deps, argv } = recordingSetup();
		await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" });
		expect(argv).toEqual([GOLDEN_ARGV]);
	});

	test("never shows the invite code in the result", async () => {
		const { deps } = setup();
		const result = await spawnWorker(deps, { space: "demo", name: "worker", cwd: "/w" });
		expect(JSON.stringify(result)).not.toContain(CODE);
	});
});
