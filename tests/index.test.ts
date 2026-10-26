import { describe, expect, test } from "bun:test";
import { readFile, stat, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import type { BusExtensionOptions } from "../extension/index.ts";
import { createCredentialStore } from "../extension/lib/session/stored-credentials.ts";
import { formatEntry } from "../extension/lib/format.ts";
import type { RunOptions, Runner } from "../extension/lib/runner.ts";
import {
	boot,
	type BootSession,
	CODE,
	callsTo,
	entry,
	errorResponse,
	fakeBus,
	type FakeBusOptions,
	frame,
	memberBus,
	streamOf,
	type ToolHandler,
	URL_,
	useTempDirs,
} from "./fakes.ts";

const toolListAuth = (bus: Parameters<typeof callsTo>[0]) => callsTo(bus, "/v1/tools").map(c => c.auth);

describe("inbox streaming", () => {
	const tempDir = useTempDirs();

	/** Boots with env credentials, runs session_start until the scripted streams run out (the final 401 warning), then shuts down. */
	async function runSession(mode: string, options: FakeBusOptions = {}) {
		const streams = options.streams ?? [() => new Response(streamOf([frame(entry(1)), frame(entry(2, { critical: true }))]))];
		const bus = fakeBus({ ...options, streams });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode });
		await app.emit("session_start");
		await app.warning("invalid token");
		await app.emit("session_shutdown");
		return app;
	}

	test("session_start streams the inbox: critical → steer, others → followUp", async () => {
		const { sent, warnings } = await runSession("tui");
		expect(sent).toEqual([
			{ message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true }, options: { deliverAs: "followUp", triggerTurn: true } },
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { critical: true })), display: true },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
		expect(warnings).toEqual(["agent-bus: invalid token"]);
	});

	test("another session holding the stream warns once, retries, and delivers once the stream opens", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const { sent, warnings } = await runSession("tui", {
			streams: [conflict, conflict, () => new Response(streamOf([frame(entry(1))]))],
			tools: { bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }) },
		});
		expect(warnings).toEqual(["agent-bus: another session is already connected as w; inbox not delivered here", "agent-bus: invalid token"]);
		expect(sent.map(s => s.message.content)).toEqual([formatEntry(entry(1))]);
	});

	test("only interactive modes (tui, rpc) trigger a turn; one-shot modes (print, json) do not", async () => {
		for (const [mode, triggerTurn] of [
			["print", false],
			["json", false],
			["tui", true],
			["rpc", true],
		] as const) {
			const { sent } = await runSession(mode);
			expect({ mode, options: sent.map(s => s.options) }).toEqual({
				mode,
				options: [
					{ deliverAs: "followUp", triggerTurn },
					{ deliverAs: "steer", triggerTurn },
				],
			});
		}
	});

	test("promote streams as the minted leader without writing its token to credentials.json; a new session forgets it", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, adminToken: "adm" });
		const bus = fakeBus({
			streams: [() => new Response(streamOf([frame(entry(1))]))],
			tools: { bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }) },
		});
		const app = await boot({ fetchFn: bus.fetchFn, env });
		await app.emit("session_start");
		await app.run("promote lead");
		await app.warning("invalid token");
		await app.emit("session_shutdown");
		await app.emit("session_start");
		expect(callsTo(bus, "/v1/inbox/stream").map(c => c.auth)).toEqual(["Bearer lead-tok", "Bearer lead-tok"]);
		expect(await store.load()).toEqual({ url: URL_, adminToken: "adm" });
	});
});

describe("accepted identity", () => {
	const tempDir = useTempDirs();

	async function worker(session: BootSession = {}, bus = memberBus()) {
		const dir = await tempDir();
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: dir } }, session);
		return { app, bus, dir };
	}

	test("/bus accept stores {url, token} in <dir>/sessions/<key>.json and switches the tools on with the member token", async () => {
		const dir = await tempDir();
		const bus = memberBus();
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: dir } }, { sessionFile: join(dir, "s1.jsonl") });
		await app.emit("session_start");
		expect(app.active()).toEqual(["read", "bus_foreign"]);
		await app.run(`accept ${URL_}/ ${CODE}`);
		const out = await app.tool("bus_list_spaces").execute("id", {});
		await app.emit("session_shutdown");
		expect(app.notes).toEqual(["Joined space demo as member worker."]);
		expect(callsTo(bus, "/v1/invites/accept")).toMatchObject([{ url: `${URL_}/v1/invites/accept`, auth: `Bearer ${CODE}` }]);
		expect(JSON.parse(await readFile(join(dir, "sessions", "s1.json"), "utf8"))).toEqual({ url: URL_, token: "acc-tok" });
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_list_spaces", "bus_spawn_worker"]);
		expect(out.content[0].text).toBe(JSON.stringify("Bearer acc-tok", null, 2));
	});

	test("/bus accept without a session file joins for this session only and says so", async () => {
		const { app, bus, dir } = await worker();
		await app.run(`accept ${URL_} ${CODE}`);
		await app.emit("session_shutdown");
		expect(app.notes).toEqual(["Joined space demo as member worker. Not persisted: this session has no session file."]);
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
		await expect(stat(join(dir, "sessions"))).rejects.toThrow();
	});

	test("the first prompt that travelled with the invite is sent once the session has joined", async () => {
		const { app } = await worker({}, memberBus({ task: "fix the bug" }));
		await app.run(`accept ${URL_} ${CODE}`);
		await app.emit("session_shutdown");
		expect(app.userMessages).toEqual(["fix the bug"]);
		expect(app.active()).toContain("bus_list_spaces");
	});

	test("a resumed session (same session file) uses the accepted token without accepting again", async () => {
		const dir = await tempDir();
		const env = { OMP_AGENT_BUS_DIR: dir };
		const sessionFile = join(dir, "s1.jsonl");
		const first = await boot({ fetchFn: memberBus().fetchFn, env }, { sessionFile });
		await first.run(`accept ${URL_} ${CODE}`);
		await first.emit("session_shutdown");
		const bus = memberBus();
		const resumed = await boot({ fetchFn: bus.fetchFn, env }, { sessionFile });
		await resumed.emit("session_start");
		await resumed.emit("session_shutdown");
		expect(callsTo(bus, "/v1/invites/accept")).toEqual([]);
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
	});

	test("a subagent whose session file is <root>/<id>.jsonl acts with the token its root session accepted", async () => {
		const dir = await tempDir();
		const env = { OMP_AGENT_BUS_DIR: dir };
		const root = join(dir, "root.jsonl");
		await writeFile(root, "");
		const main = await boot({ fetchFn: memberBus().fetchFn, env }, { sessionFile: root });
		await main.run(`accept ${URL_} ${CODE}`);
		await main.emit("session_shutdown");
		const bus = memberBus();
		const sub = await boot({ fetchFn: bus.fetchFn, env }, { kind: "sub", sessionFile: join(dir, "root", "task-1.jsonl") });
		await sub.emit("session_start");
		expect(callsTo(bus, "/v1/invites/accept")).toEqual([]);
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
	});

	test("session_switch to a new session drops the accepted token and deactivates exactly the extension's bus tools", async () => {
		const session: BootSession = {};
		const { app, dir } = await worker(session);
		session.sessionFile = join(dir, "s1.jsonl");
		await app.run(`accept ${URL_} ${CODE}`);
		session.sessionFile = join(dir, "s2.jsonl");
		await app.emit("session_switch", { type: "session_switch", reason: "new", previousSessionFile: join(dir, "s1.jsonl") });
		expect(app.active()).toEqual(["read", "bus_foreign"]);
		await expect(app.tool("bus_list_spaces").execute("id", {})).rejects.toThrow("Not logged in");
		await app.run("status");
		await app.emit("session_shutdown");
		expect(app.notes[1]).toStartWith("Not logged in.");
	});

	test("session_switch back to a worker's session reconnects with its accepted token and re-activates the tools", async () => {
		const session: BootSession = {};
		const { app, bus, dir } = await worker(session);
		session.sessionFile = join(dir, "w.jsonl");
		await app.run(`accept ${URL_} ${CODE}`);
		session.sessionFile = undefined;
		await app.emit("session_switch", { type: "session_switch", reason: "new", previousSessionFile: join(dir, "w.jsonl") });
		session.sessionFile = join(dir, "w.jsonl");
		await app.emit("session_switch", { type: "session_switch", reason: "resume", previousSessionFile: undefined });
		const resumed = await app.tool("bus_list_spaces").execute("id", {});
		await app.emit("session_shutdown");
		expect(resumed.content[0].text).toBe(JSON.stringify("Bearer acc-tok"));
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_list_spaces", "bus_spawn_worker"]);
		expect(callsTo(bus, "/v1/inbox/stream").every(c => c.auth === "Bearer acc-tok")).toBe(true);
	});

	test("session_branch and a fork move the worker onto a new session file and carry its accepted token, not the stored leader's", async () => {
		const session: BootSession = {};
		const { app, bus, dir } = await worker(session);
		await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, token: "lead" });
		session.sessionFile = join(dir, "w.jsonl");
		await app.run(`accept ${URL_} ${CODE}`);
		session.sessionFile = join(dir, "b.jsonl");
		await app.emit("session_branch", { type: "session_branch", reason: "branch", previousSessionFile: join(dir, "w.jsonl") });
		const branched = await app.tool("bus_list_spaces").execute("id", {});
		session.sessionFile = join(dir, "f.jsonl");
		await app.emit("session_switch", { type: "session_switch", reason: "fork", previousSessionFile: join(dir, "b.jsonl") });
		const forked = await app.tool("bus_list_spaces").execute("id", {});
		await app.emit("session_shutdown");
		expect([branched.content[0].text, forked.content[0].text]).toEqual([JSON.stringify("Bearer acc-tok"), JSON.stringify("Bearer acc-tok")]);
		expect(JSON.parse(await readFile(join(dir, "sessions", "f.json"), "utf8"))).toEqual({ url: URL_, token: "acc-tok" });
		expect(callsTo(bus, "/v1/inbox/stream").every(c => c.auth === "Bearer acc-tok")).toBe(true);
	});

	test("--bus-accept with --bus-url is redeemed on session_start, sends the invite's task, and is not redeemed again", async () => {
		const session: BootSession = { flags: { "bus-url": URL_, "bus-accept": CODE } };
		const { app, bus, dir } = await worker(session, memberBus({ task: "fix the bug" }));
		session.sessionFile = join(dir, "w.jsonl");
		await app.emit("session_start");
		await app.emit("session_switch", { type: "session_switch", reason: "new", previousSessionFile: undefined });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(callsTo(bus, "/v1/invites/accept")).toHaveLength(1);
		expect(app.notes).toEqual(["Joined space demo as member worker."]);
		expect(app.userMessages).toEqual(["fix the bug"]);
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
	});

	test("--bus-accept falls back to the stored url", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: `${URL_}/`, adminToken: "adm" });
		const bus = memberBus();
		const app = await boot({ fetchFn: bus.fetchFn, env }, { flags: { "bus-accept": CODE } });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(callsTo(bus, "/v1/invites/accept")).toMatchObject([{ url: `${URL_}/v1/invites/accept` }]);
	});

	test("a failed --bus-accept is warned and the session still starts with its other credentials", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: URL_, token: "mem" });
		const bus = memberBus();
		const app = await boot({ fetchFn: bus.fetchFn, env }, { flags: { "bus-url": URL_, "bus-accept": "nope-nope" } });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.warnings[0]).toBe("agent-bus: unauthenticated: invalid or expired invite code");
		expect(toolListAuth(bus)).toEqual(["Bearer mem"]);
	});

	test("logout deletes the session's accepted token file", async () => {
		const session: BootSession = {};
		const { app, dir } = await worker(session);
		session.sessionFile = join(dir, "s1.jsonl");
		await app.run(`accept ${URL_} ${CODE}`);
		await app.run("logout");
		expect(app.notes[1]).toBe("Logged out; bus tools deactivated.");
		await expect(stat(join(dir, "sessions", "s1.json"))).rejects.toThrow();
		expect(app.active()).toEqual(["read"]);
	});

	test("an accept error is shown as <code>: <message> and stores nothing", async () => {
		const session: BootSession = {};
		const { app, dir } = await worker(session);
		session.sessionFile = join(dir, "s1.jsonl");
		await app.run(`accept ${URL_} nope-nope`);
		expect(app.notes).toEqual(["unauthenticated: invalid or expired invite code"]);
		await expect(stat(join(dir, "sessions"))).rejects.toThrow();
	});
});

describe("tool activation and credentials", () => {
	const tempDir = useTempDirs();

	test("bus_spawn_worker is registered inactive and switched on only once credentials resolve", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const app = await boot({ fetchFn: memberBus().fetchFn, env });
		await app.emit("session_start");
		expect(app.tool("bus_spawn_worker").defaultInactive).toBe(true);
		expect(app.active()).toEqual(["read", "bus_foreign"]);
		await app.run(`login ${URL_} mem`);
		await app.emit("session_shutdown");
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_list_spaces", "bus_spawn_worker"]);
	});

	test("logout deactivates every bus_* tool and keeps the others; calls fail until login re-activates them with the new token", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: URL_, token: "mem" });
		const app = await boot({ fetchFn: memberBus().fetchFn, env });
		await app.emit("session_start");
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_list_spaces", "bus_spawn_worker"]);
		await app.run("logout");
		expect(app.active()).toEqual(["read"]);
		await expect(app.tool("bus_list_spaces").execute("id", {})).rejects.toThrow("Not logged in to the agent bus. Run /bus login <url> <token>.");
		await app.run(`login ${URL_} mem2`);
		expect(app.active()).toEqual(["read", "bus_list_spaces", "bus_spawn_worker"]);
		const out = await app.tool("bus_list_spaces").execute("id", {});
		await app.emit("session_shutdown");
		expect(out.content[0].text).toBe(JSON.stringify("Bearer mem2", null, 2));
		expect(app.tools.map(t => t.name)).toEqual(["bus_spawn_worker", "bus_list_spaces"]);
		expect(app.notes).toEqual(["Logged out; bus tools deactivated.", "Logged in as member w"]);
	});

	test("after a logout, /resume of a worker's session switches every bus tool back on", async () => {
		const session: BootSession = {};
		const dir = await tempDir();
		const app = await boot({ fetchFn: memberBus().fetchFn, env: { OMP_AGENT_BUS_DIR: dir } }, session);
		session.sessionFile = join(dir, "w.jsonl");
		await app.run(`accept ${URL_} ${CODE}`);
		session.sessionFile = join(dir, "other.jsonl");
		await app.emit("session_switch", { type: "session_switch", reason: "new", previousSessionFile: join(dir, "w.jsonl") });
		await app.run("logout");
		session.sessionFile = join(dir, "w.jsonl");
		await app.emit("session_switch", { type: "session_switch", reason: "resume", previousSessionFile: join(dir, "other.jsonl") });
		await app.emit("session_shutdown");
		expect(app.active()).toEqual(["read", "bus_list_spaces", "bus_spawn_worker"]);
	});

	test("an admin login without agent credentials switches no bus tool on", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const bus = fakeBus({ tools: { bus_whoami: () => Response.json({ result: { kind: "admin", spaces: [] } }) } });
		const app = await boot({ fetchFn: bus.fetchFn, env });
		await app.emit("session_start");
		await app.run(`login ${URL_} adm`);
		await app.emit("session_shutdown");
		expect(app.notes).toEqual(["Admin token stored. Run /bus promote <leader-name> to act as a leader."]);
		expect(app.active()).toEqual(["read", "bus_foreign"]);
	});

	test("env credentials are deleted from the environment at load, used from memory, and logout leaves no env fallback", async () => {
		const env: NodeJS.ProcessEnv = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: `${URL_}/`, AGENT_BUS_TOKEN: "env-tok", AGENT_BUS_BIN: "/opt/amb" };
		const bus = memberBus();
		const app = await boot({ fetchFn: bus.fetchFn, env });
		expect(env).toEqual({ OMP_AGENT_BUS_DIR: env.OMP_AGENT_BUS_DIR });
		await app.emit("session_start");
		expect(toolListAuth(bus)).toEqual(["Bearer env-tok"]);
		await app.run("logout");
		await expect(app.tool("bus_list_spaces").execute("id", {})).rejects.toThrow("Not logged in");
		await app.run("status");
		expect(app.notes[1]).toStartWith("Not logged in.");
	});

	test("the default environment is process.env, and the bus variables are deleted from it at load", async () => {
		const saved = { ...process.env };
		try {
			Object.assign(process.env, { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok", AGENT_BUS_BIN: "/opt/amb" });
			const bus = memberBus();
			const app = await boot({ fetchFn: bus.fetchFn });
			expect([process.env.AGENT_BUS_URL, process.env.AGENT_BUS_TOKEN, process.env.AGENT_BUS_BIN]).toEqual([undefined, undefined, undefined]);
			await app.emit("session_start");
			expect(toolListAuth(bus)).toEqual(["Bearer env-tok"]);
			await app.emit("session_shutdown");
		} finally {
			for (const key of Object.keys(process.env)) if (!(key in saved)) delete process.env[key];
			Object.assign(process.env, saved);
		}
	});

	test("a subagent session gets no inbox loop and no inbox tools; the other bus tools register; a main session gets both", async () => {
		const schema = { type: "object" as const, properties: {} };
		const list = ["bus_inbox_read", "bus_send", "bus_inbox_ack"].map(name => ({ name, description: name, inputSchema: schema }));
		const results: Record<string, { tools: string[]; streamed: boolean }> = {};
		for (const kind of ["sub", "main"]) {
			const bus = fakeBus({ list });
			const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" } }, { kind });
			await app.emit("session_start");
			await app.emit("session_shutdown");
			results[kind] = { tools: app.tools.map(t => t.name), streamed: callsTo(bus, "/v1/inbox/stream").length > 0 };
		}
		expect(results).toEqual({
			sub: { tools: ["bus_spawn_worker", "bus_send"], streamed: false },
			main: { tools: ["bus_spawn_worker", "bus_inbox_read", "bus_send", "bus_inbox_ack"], streamed: true },
		});
	});
});

describe("worker spawn", () => {
	const tempDir = useTempDirs();
	const inviteOk: ToolHandler = args => Response.json({ result: { name: args.name, space: args.space, code: CODE, expires_at: "x" } });

	/** A started leader session (stored token `lead`) inside tmux whose bus answers `bus_invite_member` with `invite`. */
	async function leaderInTmux(invite: ToolHandler = inviteOk, options: BusExtensionOptions = {}) {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), TMUX: "/tmp/tmux-1000/default,1,0" };
		await createCredentialStore(env).save({ url: URL_, token: "lead" });
		const bus = fakeBus({ tools: { bus_invite_member: invite } });
		const runs: { argv: string[]; options?: RunOptions }[] = [];
		const runner: Runner = async (command, args, runOptions) => {
			runs.push({ argv: [command, ...args], options: runOptions });
			return { code: 0, stdout: "", stderr: "" };
		};
		const app = await boot({ fetchFn: bus.fetchFn, env, runner, platform: "linux", which: async () => undefined, argv: [], ...options });
		await app.emit("session_start");
		return { app, invites: () => callsTo(bus, "/bus_invite_member"), runs };
	}
	const tmuxRun = (cwd: string, ...plugin: string[]) => ["tmux", "new-window", "-n", "worker", "--", "omp", ...plugin, "--cwd", cwd, "--bus-url", URL_, "--bus-accept", CODE];

	test("/bus spawn invites the member with its task and the active token, and opens a tmux window", async () => {
		const { app, invites, runs } = await leaderInTmux();
		await app.run("spawn demo worker fix the bug");
		await app.emit("session_shutdown");
		expect(invites()).toMatchObject([{ url: `${URL_}/v1/tools/bus_invite_member`, auth: "Bearer lead", body: { space: "demo", name: "worker", task: "fix the bug" } }]);
		expect(runs.map(r => r.argv)).toEqual([tmuxRun("/w")]);
		expect(app.notes).toEqual(["Started worker worker in space demo (tmux)."]);
	});

	test("the worker gets the leader's --plugin-dir and -e flags", async () => {
		const { app, runs } = await leaderInTmux(inviteOk, { argv: ["bun", "omp", "--plugin-dir", "/amb", "-e", "/amb/x.ts", "--model", "m"] });
		await app.run("spawn demo worker");
		await app.emit("session_shutdown");
		expect(runs.map(r => r.argv)).toEqual([tmuxRun("/w", "--plugin-dir", "/amb", "-e", "/amb/x.ts")]);
	});

	test("bus_spawn_worker resolves cwd against the session's, passes its abort signal to the launcher, and maps bus errors", async () => {
		let denied = false;
		const { app, runs } = await leaderInTmux((args, auth) => (denied ? errorResponse(403, "forbidden", "only leaders can invite") : inviteOk(args, auth)));
		const tool = app.tool("bus_spawn_worker");
		const signal = new AbortController().signal;
		const out = await tool.execute("id", { space: "demo", name: "worker", cwd: "server" }, signal, undefined, { cwd: "/w" });
		await tool.execute("id", { space: "demo", name: "worker" }, undefined, undefined, { cwd: "/w" });
		expect(JSON.parse(out.content[0].text)).toEqual({ name: "worker", space: "demo", launcher: "tmux" });
		expect(runs.map(r => r.argv)).toEqual([tmuxRun(resolve("/w", "server")), tmuxRun("/w")]);
		expect(runs[0].options).toEqual({ timeoutMs: 15_000, signal });
		expect(out.content[0].text).not.toContain(CODE);
		denied = true;
		await expect(tool.execute("id", { space: "demo", name: "worker" }, undefined, undefined, { cwd: "/w" })).rejects.toThrow("forbidden: only leaders can invite");
		await app.emit("session_shutdown");
	});
});

describe("local server", () => {
	const tempDir = useTempDirs();

	/** Fake agent-message-bus CLI: records each run and answers start, status --json, admin-token, stop and status. */
	function fakeCli(running = true) {
		const runs: string[][] = [];
		const out: Record<string, string> = {
			start: "agent-message-bus started (pid 7) at http://127.0.0.1:8080; log: /x.log\n",
			"status --json": JSON.stringify({ running, pid: 7, url: "http://127.0.0.1:8080" }),
			"admin-token": "amb_admin_secret\n",
			stop: "agent-message-bus stopped (pid 7)\n",
			status: "running (pid 7) at http://127.0.0.1:8080\n",
		};
		const runner: Runner = async (command, args) => {
			runs.push([command, ...args]);
			return { code: 0, stdout: out[args.join(" ")] ?? "", stderr: "" };
		};
		return { runs, runner };
	}

	test("/bus server start runs start, status --json, admin-token; stores the admin credentials and switches no bus tool on without agent credentials", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_BIN: "/opt/amb" };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, token: "mem" });
		const cli = fakeCli();
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: cli.runner, which: async () => "/usr/bin/agent-message-bus" });
		await app.emit("session_start");
		await app.run("logout");
		await app.run("server start");
		expect(cli.runs).toEqual([
			["/opt/amb", "start"],
			["/opt/amb", "status", "--json"],
			["/opt/amb", "admin-token"],
		]);
		expect(await store.load()).toEqual({ url: "http://127.0.0.1:8080", adminToken: "amb_admin_secret" });
		expect(app.active()).toEqual(["read"]);
		expect(app.notes[1]).toBe("Agent message bus running at http://127.0.0.1:8080; admin token stored. Run /bus promote <leader-name> to act as a leader.");
	});

	test("/bus server start keeps a stored agent token for the server's url and switches the bus tools back on", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_BIN: "/opt/amb" };
		const store = createCredentialStore(env);
		await store.save({ url: "http://127.0.0.1:8080", token: "mem" });
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: fakeCli().runner });
		await app.emit("session_start");
		const tools = app.active();
		await app.run("logout");
		await store.save({ url: "http://127.0.0.1:8080", token: "mem" });
		await app.run("server start");
		expect(app.active()).toEqual(tools.filter(name => name !== "bus_foreign"));
		expect(tools).toContain("bus_spawn_worker");
	});

	test("/bus server stop runs stop and logs out; /bus server status prints the CLI status", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, token: "mem" });
		const cli = fakeCli();
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: cli.runner, which: async () => "/usr/bin/agent-message-bus" });
		await app.emit("session_start");
		await app.run("server stop");
		await app.run("server status");
		expect(cli.runs).toEqual([
			["/usr/bin/agent-message-bus", "stop"],
			["/usr/bin/agent-message-bus", "status"],
		]);
		expect(await store.load()).toBeUndefined();
		expect(app.active()).toEqual(["read"]);
		expect(app.notes).toEqual(["agent-message-bus stopped (pid 7)\nLogged out; bus tools deactivated.", "running (pid 7) at http://127.0.0.1:8080"]);
	});

	test("/bus server start fails when the CLI reports no running instance, or without a subcommand shows its usage", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_BIN: "/opt/amb" };
		const app = await boot({ fetchFn: memberBus().fetchFn, env, runner: fakeCli(false).runner });
		await app.run("server start");
		await app.run("server");
		expect(app.notes).toEqual(["agent-message-bus started but reports no running instance", "Usage: /bus server start|stop|status"]);
		expect(await createCredentialStore(env).load()).toBeUndefined();
	});
});
