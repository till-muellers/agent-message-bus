import { describe, expect, test } from "bun:test";
import { readFile, stat, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { type BusExtensionOptions, createBusExtension } from "../extension/index.ts";
import type { RemoteTool } from "../extension/lib/bus/client.ts";
import { createCredentialStore } from "../extension/lib/session/stored-credentials.ts";
import { formatEntry } from "../extension/lib/render/envelope.ts";
import type { RunOptions, Runner } from "../extension/lib/process/runner.ts";
import {
	boot,
	type Booted,
	bootExtension,
	type BootSession,
	CODE,
	callsTo,
	entry,
	errorResponse,
	fakeBus,
	type FakeBusOptions,
	frame,
	listSpaces,
	memberBus,
	noSleep,
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
		const streams = options.streams ?? [() => new Response(streamOf([frame(entry(1)), frame(entry(2, { class: "interrupt" }))]))];
		const bus = fakeBus({ ...options, streams });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode });
		await app.emit("session_start");
		await app.warning("invalid token");
		await app.emit("session_shutdown");
		return app;
	}

	test("session_start streams the inbox: the class decides the mechanism and the message carries the entry id", async () => {
		const { sent, warnings } = await runSession("tui");
		expect(sent).toEqual([
			{
				message: { customType: "agent-bus", content: formatEntry(entry(1)), display: true, details: { entryIds: [1] } },
				options: { deliverAs: "aside", triggerTurn: false },
			},
			{
				message: { customType: "agent-bus", content: formatEntry(entry(2, { class: "interrupt" })), display: true, details: { entryIds: [2] } },
				options: { deliverAs: "steer", triggerTurn: true },
			},
		]);
		expect(warnings).toEqual(["agent-bus: invalid token"]);
	});

	test("an entry is acked only after a model request carried it", async () => {
		const bus = fakeBus({ streams: [() => new Response(streamOf([frame(entry(1))]))] });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		await app.warning("invalid token");
		expect(app.sent).toHaveLength(1);
		expect(callsTo(bus, "/bus_inbox_ack")).toEqual([]);
		await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message })));
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 1 }]);
		await app.emit("session_shutdown");
	});

	test("a request without the entry acks nothing", async () => {
		const bus = fakeBus({ streams: [() => new Response(streamOf([frame(entry(1))]))] });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		await app.warning("invalid token");
		await app.context([{ role: "user", content: "hello" }]);
		expect(callsTo(bus, "/bus_inbox_ack")).toEqual([]);
		await app.emit("session_shutdown");
	});

	test("a reconnect that replays an entry does not hand it over twice", async () => {
		const bus = fakeBus({ streams: [() => new Response(streamOf([frame(entry(1))])), () => new Response(streamOf([frame(entry(1))]))] });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		await app.warning("invalid token");
		expect(app.sent).toHaveLength(1);
		await app.emit("session_shutdown");
	});

	test("a new session delivers the entry the last one did not consume again", async () => {
		const credentials = { AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), ...credentials };
		const first = fakeBus({ streams: [() => new Response(streamOf([frame(entry(1))]))] });
		const app = await boot({ fetchFn: first.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		await app.warning("invalid token");
		await app.emit("session_shutdown");
		expect(app.sent).toHaveLength(1);

		const second = fakeBus({ streams: [() => new Response(streamOf([frame(entry(1))]))] });
		// The extension reads the env credentials once and removes them, so the second session gets its own copy.
		const restarted = await boot({ fetchFn: second.fetchFn, env: { OMP_AGENT_BUS_DIR: env.OMP_AGENT_BUS_DIR, ...credentials } }, { mode: "tui" });
		await restarted.emit("session_start");
		await restarted.warning("invalid token");
		await restarted.emit("session_shutdown");
		expect(restarted.sent).toHaveLength(1);
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

	test("the class and the phase decide the mechanism: an interactive session steers and wakes, a one-shot session waits", async () => {
		const cases: [string, { deliverAs: string; triggerTurn: boolean }[]][] = [
			["print", [{ deliverAs: "nextTurn", triggerTurn: false }, { deliverAs: "nextTurn", triggerTurn: false }]],
			["json", [{ deliverAs: "nextTurn", triggerTurn: false }, { deliverAs: "nextTurn", triggerTurn: false }]],
			["tui", [{ deliverAs: "aside", triggerTurn: false }, { deliverAs: "steer", triggerTurn: true }]],
			["rpc", [{ deliverAs: "aside", triggerTurn: false }, { deliverAs: "steer", triggerTurn: true }]],
		];
		for (const [mode, options] of cases) {
			const { sent } = await runSession(mode);
			expect({ mode, options: sent.map(s => s.options) }).toEqual({ mode, options });
		}
	});

	test("promote streams as the minted leader instead of the stored admin, never writes its token, and a new session forgets it", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const store = createCredentialStore(env);
		await store.save({ url: URL_, adminToken: "adm" });
		const bus = fakeBus({ tools: { bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }) } });
		const app = await boot({ fetchFn: bus.fetchFn, env });
		await app.emit("session_start");
		await app.run("promote lead");
		await app.emit("session_shutdown");
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(callsTo(bus, "/v1/inbox/stream").map(c => c.auth)).toEqual(["Bearer adm", "Bearer lead-tok", "Bearer adm"]);
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

	test("an accept sends no user message: the task arrives as an inbox entry of the inviter", async () => {
		const { app } = await worker({}, memberBus());
		await app.run(`accept ${URL_} ${CODE}`);
		await app.emit("session_shutdown");
		expect(app.userMessages).toEqual([]);
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

	test("a server that is too old activates no bus tool and warns", async () => {
		const bus = fakeBus({ protocol: "none", list: [listSpaces] });
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" } });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.active()).toEqual(["read", "bus_foreign"]);
		expect(app.warnings).toEqual([
			`agent-bus: the server at ${URL_} is too old (protocol none, this plugin needs 2); upgrade the agent-message-bus server`,
		]);
		expect(callsTo(bus, "/v1/inbox/stream")).toEqual([]);
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

	test("--bus-accept with --bus-url is redeemed on session_start, sends no user message, and is not redeemed again", async () => {
		const session: BootSession = { flags: { "bus-url": URL_, "bus-accept": CODE } };
		const { app, bus, dir } = await worker(session, memberBus());
		session.sessionFile = join(dir, "w.jsonl");
		await app.emit("session_start");
		await app.emit("session_switch", { type: "session_switch", reason: "new", previousSessionFile: undefined });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(callsTo(bus, "/v1/invites/accept")).toHaveLength(1);
		expect(app.notes).toEqual(["Joined space demo as member worker."]);
		expect(app.userMessages).toEqual([]);
		expect(toolListAuth(bus)).toEqual(["Bearer acc-tok"]);
	});

	test("a one-shot session waits for the inbox backlog of the redeemed invite", async () => {
		const bus = memberBus({
			streams: [() => new Response(streamOf([frame(entry(5, { from: "lead", body: "fix the bug" }))]))],
			tools: { bus_inbox_read: () => Response.json({ result: [{ id: 5 }] }) },
		});
		const session: BootSession = { mode: "print", flags: { "bus-url": URL_, "bus-accept": CODE } };
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir() } }, session);
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.sent).toHaveLength(1);
		expect(app.sent[0].message.content).toContain("fix the bug");
		expect(app.warnings).not.toContain("agent-bus: inbox backlog not delivered within 5 s; continuing");
	});

	test("a one-shot session gives up on the backlog after the timeout and says so", async () => {
		const bus = memberBus({ tools: { bus_inbox_read: () => Response.json({ result: [{ id: 5 }] }) } });
		const session: BootSession = { mode: "print", flags: { "bus-url": URL_, "bus-accept": CODE } };
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir() }, backlogTimeoutMs: 50 }, session);
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.warnings).toContain("agent-bus: inbox backlog not delivered within 0.05 s; continuing");
	});

	test("an interactive session does not wait for the backlog", async () => {
		const bus = memberBus({ tools: { bus_inbox_read: () => Response.json({ result: [{ id: 5 }] }) } });
		const session: BootSession = { mode: "tui", flags: { "bus-url": URL_, "bus-accept": CODE } };
		const app = await boot({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir() }, backlogTimeoutMs: 50 }, session);
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.warnings).not.toContain("agent-bus: inbox backlog not delivered within 0.05 s; continuing");
	});

	test("a one-shot session with stored credentials does not wait for the backlog", async () => {
		const bus = memberBus({ tools: { bus_inbox_read: () => Response.json({ result: [{ id: 5 }] }) } });
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: URL_, token: "acc-tok" });
		const app = await boot({ fetchFn: bus.fetchFn, env, backlogTimeoutMs: 50 }, { mode: "print" });
		await app.emit("session_start");
		await app.emit("session_shutdown");
		expect(app.warnings).not.toContain("agent-bus: inbox backlog not delivered within 0.05 s; continuing");
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

	test("an admin-only login is a full agent session of that admin: its tools are active and its inbox stream opens", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		const schema = { type: "object" as const, properties: {} };
		const list = ["bus_mint_admin", "bus_list_admins", "bus_revoke_admin", "bus_inbox_read", "bus_inbox_ack"].map(name => ({ name, description: name, inputSchema: schema }));
		const bus = fakeBus({ list, tools: { bus_whoami: () => Response.json({ result: { kind: "admin", name: "ops", spaces: [] } }) } });
		const app = await boot({ fetchFn: bus.fetchFn, env });
		await app.emit("session_start");
		await app.run(`login ${URL_} adm`);
		await app.emit("session_shutdown");
		expect(app.notes).toEqual(["Admin token stored. Run /bus promote <leader-name> to act as a leader."]);
		expect(app.active()).toEqual(["read", "bus_foreign", "bus_mint_admin", "bus_list_admins", "bus_revoke_admin", "bus_inbox_read", "bus_spawn_worker"]);
		expect(callsTo(bus, "/v1/inbox/stream").map(c => c.auth)).toEqual(["Bearer adm"]);
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
});

describe("subagent identity", () => {
	const tempDir = useTempDirs();
	const addPost: RemoteTool = { name: "bus_add_post", description: "Post", inputSchema: { type: "object", properties: {} } };
	const minted: ToolHandler = args => Response.json({ result: { name: args.name, token: `${args.name}-tok` } });
	const whoami: ToolHandler = () => Response.json({ result: { kind: "leader", name: "lead2", spaces: [] } });

	/** A bus whose bus_add_post answers with the caller's bearer. */
	const postBus = () => fakeBus({ list: [addPost], tools: { bus_add_post: (_args, auth) => Response.json({ result: auth }), bus_mint_leader: minted, bus_whoami: whoami } });

	/**
	 * A main session on `<dir>/root.jsonl` and, from the same factory, starters for its subagents (`<dir>/root/<id>.jsonl`).
	 * The root file is never written: omp writes it only after the first assistant message, often after a subagent started.
	 */
	async function family(env: NodeJS.ProcessEnv, bus = postBus()) {
		const dir = env.OMP_AGENT_BUS_DIR ?? "";
		const extension = createBusExtension({ fetchFn: bus.fetchFn, env, sleep: noSleep });
		const main = await bootExtension(extension, { mode: "tui", sessionFile: join(dir, "root.jsonl") });
		const startSub = async () => {
			const sub = await bootExtension(extension, { kind: "sub", agentId: "Explore", sessionFile: join(dir, "root", "task-1.jsonl") });
			await sub.emit("session_start");
			return sub;
		};
		return { main, startSub, extension };
	}

	const post = async (app: Booted) =>
		JSON.parse((await app.tool("bus_add_post").execute("id", { body: "tests pass" })).content[0].text) as string;

	test("the subagent posts with the identity its main session acts with, however the main session got it", async () => {
		const hows: Record<string, (dir: string) => Promise<{ env: NodeJS.ProcessEnv; start(main: Booted): Promise<void> }>> = {
			"by accepting an invite": async dir => ({ env: { OMP_AGENT_BUS_DIR: dir }, start: main => main.run(`accept ${URL_} ${CODE}`) }),
			"from AGENT_BUS_URL + AGENT_BUS_TOKEN": async dir => ({ env: { OMP_AGENT_BUS_DIR: dir, AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" }, start: main => main.emit("session_start") }),
			"from /bus promote": async dir => {
				await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, adminToken: "adm" });
				return { env: { OMP_AGENT_BUS_DIR: dir }, start: main => main.run("promote lead2") };
			},
			"from the stored agent token": async dir => {
				await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, token: "stored-tok" });
				return { env: { OMP_AGENT_BUS_DIR: dir }, start: main => main.emit("session_start") };
			},
		};
		const authors: Record<string, string> = {};
		for (const [how, setUp] of Object.entries(hows)) {
			const { env, start } = await setUp(await tempDir());
			const { main, startSub } = await family(env);
			await start(main);
			const sub = await startSub();
			authors[how] = await post(sub);
			await main.emit("session_shutdown");
		}
		expect(authors).toEqual({
			"by accepting an invite": "Bearer acc-tok",
			"from AGENT_BUS_URL + AGENT_BUS_TOKEN": "Bearer env-tok",
			"from /bus promote": "Bearer lead2-tok",
			"from the stored agent token": "Bearer stored-tok",
		});
	});

	test("a subagent gets the five blackboard tools at most and no inbox stream; the main session gets every bus tool", async () => {
		const schema = { type: "object" as const, properties: {} };
		const list = ["bus_inbox_read", "bus_send", "bus_inbox_ack", "bus_read_thread", "bus_add_post"].map(name => ({ name, description: name, inputSchema: schema }));
		const bus = fakeBus({ list });
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" }, bus);
		await main.emit("session_start");
		const streams = callsTo(bus, "/v1/inbox/stream").length;
		const sub = await startSub();
		await sub.emit("session_shutdown");
		await main.emit("session_shutdown");
		expect(sub.tools.map(t => t.name)).toEqual(["bus_read_thread", "bus_add_post"]);
		expect(callsTo(bus, "/v1/inbox/stream").length).toBe(streams);
		expect(main.tools.map(t => t.name)).toEqual(["bus_spawn_worker", "bus_inbox_read", "bus_send", "bus_read_thread", "bus_add_post"]);
	});

	test("a subagent that ends leaves its main session's identity to the next subagent", async () => {
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" });
		await main.emit("session_start");
		await (await startSub()).emit("session_shutdown");
		const next = await startSub();
		expect(await post(next)).toBe("Bearer env-tok");
		await main.emit("session_shutdown");
		await expect(post(next)).rejects.toThrow("Not logged in to the agent bus.");
	});

	test("a session switch moves the identity to the new session's subagents and away from the old one's", async () => {
		const dir = await tempDir();
		const bus = postBus();
		const extension = createBusExtension({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: dir, AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" }, sleep: noSleep });
		const session: BootSession = { mode: "tui", sessionFile: join(dir, "first.jsonl") };
		const main = await bootExtension(extension, session);
		await main.emit("session_start");
		const old = await bootExtension(extension, { kind: "sub", sessionFile: join(dir, "first", "task-1.jsonl") });
		await old.emit("session_start");
		session.sessionFile = join(dir, "second.jsonl");
		await main.emit("session_switch", { reason: "new" });
		const fresh = await bootExtension(extension, { kind: "sub", sessionFile: join(dir, "second", "task-1.jsonl") });
		await fresh.emit("session_start");
		await expect(post(old)).rejects.toThrow("Not logged in to the agent bus.");
		expect(await post(fresh)).toBe("Bearer env-tok");
		await main.emit("session_shutdown");
	});

	test("after the main session logs out, the subagent's next call fails as not logged in", async () => {
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" });
		await main.emit("session_start");
		const sub = await startSub();
		await main.run("logout");
		await expect(post(sub)).rejects.toThrow("Not logged in to the agent bus. Run /bus login <url> <token>.");
	});

	test("a later /bus promote in the main session reaches the subagent's next call", async () => {
		const dir = await tempDir();
		await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, adminToken: "adm", token: "worker-tok" });
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: dir });
		await main.emit("session_start");
		const sub = await startSub();
		const before = await post(sub);
		await main.run("promote lead2");
		const after = await post(sub);
		await main.emit("session_shutdown");
		expect([before, after]).toEqual(["Bearer worker-tok", "Bearer lead2-tok"]);
	});

	test("without credentials in the main session the subagent gets no bus tool and says so once", async () => {
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: await tempDir() });
		await main.emit("session_start");
		const sub = await startSub();
		await sub.emit("session_start");
		expect(sub.tools.map(t => t.name)).toEqual([]);
		expect(sub.warnings).toEqual(["agent-bus: no bus tools for the subagent: its main session has no bus identity"]);
	});

	test("a subagent never falls back to a stored identity: a main session without a session file gives it none", async () => {
		const dir = await tempDir();
		await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, token: "worker-tok" });
		const bus = postBus();
		const extension = createBusExtension({ fetchFn: bus.fetchFn, env: { OMP_AGENT_BUS_DIR: dir }, sleep: noSleep });
		const main = await bootExtension(extension, { mode: "tui" });
		await main.emit("session_start");
		const sub = await bootExtension(extension, { kind: "sub", sessionFile: join(dir, "root", "task-1.jsonl") });
		await sub.emit("session_start");
		await main.emit("session_shutdown");
		expect(sub.tools.map(t => t.name)).toEqual([]);
		expect(toolListAuth(bus)).toEqual(["Bearer worker-tok"]);
	});

	test("a subagent under another session's dir gets nothing from this main session", async () => {
		const dir = await tempDir();
		const { main, extension } = await family({ OMP_AGENT_BUS_DIR: dir, AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" });
		await main.emit("session_start");
		const other = await bootExtension(extension, { kind: "sub", sessionFile: join(dir, "other", "task-1.jsonl") });
		await other.emit("session_start");
		await main.emit("session_shutdown");
		expect(other.tools.map(t => t.name)).toEqual([]);
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
			// `omp plugin list --json` reports the installed plugin; the launcher runs answer with nothing.
			if (args[0] === "plugin") return { code: 0, stdout: JSON.stringify({ npm: [], marketplace: [] }), stderr: "" };
			return { code: 0, stdout: "", stderr: "" };
		};
		const app = await boot({ fetchFn: bus.fetchFn, env, runner, platform: "linux", which: async () => undefined, argv: [], ...options });
		await app.emit("session_start");
		return { app, invites: () => callsTo(bus, "/bus_invite_member"), runs };
	}
	const tmuxRun = (cwd: string, ...plugin: string[]) => ["tmux", "new-window", "-n", "worker", "--", "omp", ...plugin, "--cwd", cwd, "--bus-url", URL_, "--bus-accept", CODE];
	/** The launcher runs, without the `omp plugin list` of the version check. */
	const launches = (runs: { argv: string[]; options?: RunOptions }[]) => runs.filter(run => run.argv[0] !== "omp");

	test("/bus spawn invites the member with its task and the active token, and opens a tmux window", async () => {
		const { app, invites, runs } = await leaderInTmux();
		await app.run("spawn demo worker fix the bug");
		await app.emit("session_shutdown");
		expect(invites()).toMatchObject([{ url: `${URL_}/v1/tools/bus_invite_member`, auth: "Bearer lead", body: { space: "demo", name: "worker", task: "fix the bug" } }]);
		expect(launches(runs).map(run => run.argv)).toEqual([tmuxRun("/w")]);
		expect(app.notes).toEqual(["Started worker worker in space demo (tmux)."]);
	});

	test("the worker gets the leader's --plugin-dir and -e flags", async () => {
		const { app, runs } = await leaderInTmux(inviteOk, { argv: ["bun", "omp", "--plugin-dir", "/amb", "-e", "/amb/x.ts", "--model", "m"] });
		await app.run("spawn demo worker");
		await app.emit("session_shutdown");
		expect(launches(runs).map(run => run.argv)).toEqual([tmuxRun("/w", "--plugin-dir", "/amb", "-e", "/amb/x.ts")]);
	});

	test("bus_spawn_worker resolves cwd against the session's, passes its abort signal to the launcher, and maps bus errors", async () => {
		let denied = false;
		const { app, runs } = await leaderInTmux((args, auth) => (denied ? errorResponse(403, "forbidden", "only leaders can invite") : inviteOk(args, auth)));
		const tool = app.tool("bus_spawn_worker");
		const signal = new AbortController().signal;
		const out = await tool.execute("id", { space: "demo", name: "worker", cwd: "server" }, signal, undefined, { cwd: "/w" });
		await tool.execute("id", { space: "demo", name: "worker" }, undefined, undefined, { cwd: "/w" });
		expect(JSON.parse(out.content[0].text)).toEqual({ name: "worker", space: "demo", launcher: "tmux" });
		expect(launches(runs).map(run => run.argv)).toEqual([tmuxRun(resolve("/w", "server")), tmuxRun("/w")]);
		expect(launches(runs)[0].options).toEqual({ timeoutMs: 15_000, signal });
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

	test("/bus server start runs start, status --json, admin-token; stores the admin credentials and acts as that admin without agent credentials", async () => {
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
		expect(app.active()).toEqual(["read", "bus_list_spaces", "bus_spawn_worker"]);
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
