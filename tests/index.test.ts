import { describe, expect, jest, test } from "bun:test";
import { readFile, stat, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { type BusExtensionOptions, createBusExtension } from "../extension/index.ts";
import type { RemoteTool } from "../extension/lib/bus/client.ts";
import { createCredentialStore } from "../extension/lib/session/stored-credentials.ts";
import { envelope } from "../extension/lib/render/envelope.ts";
import { busMessage } from "../extension/lib/render/details.ts";
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
	fakeEvents,
	type FakeComponent,
	frame,
	listSpaces,
	liveStream,
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

	test("session_start streams the inbox: the class decides the mechanism and the message carries the entry", async () => {
		const { sent, warnings } = await runSession("tui");
		expect(sent).toEqual([
			{ message: busMessage(entry(1)), options: { deliverAs: "aside", triggerTurn: false } },
			{ message: busMessage(entry(2, { class: "interrupt" })), options: { deliverAs: "steer", triggerTurn: true } },
		]);
		expect(sent[0].message).toMatchObject({ customType: "agent-bus", content: envelope(entry(1)), display: true, details: { entryIds: [1] } });
		expect(warnings).toEqual(["agent-bus: invalid token"]);
	});

	test("non-TUI modes get the same plain envelope text", async () => {
		for (const mode of ["rpc", "print"]) {
			const { sent } = await runSession(mode);
			expect({ mode, content: sent.map(s => s.message.content) }).toEqual({ mode, content: [envelope(entry(1)), envelope(entry(2, { class: "interrupt" }))] });
		}
	});

	test("agent-bus messages render as entry cards from their details, so a resumed session shows the same card", async () => {
		const { sent, renderers } = await runSession("tui");
		const render = renderers.get("agent-bus");
		if (!render) throw new Error("no agent-bus renderer");
		const theme = { fg: (_color: string, text: string) => text, bold: (text: string) => text } as unknown as Parameters<typeof render>[2];
		const stored = JSON.parse(JSON.stringify({ role: "custom", ...sent[0].message, timestamp: 1 }));
		const card = render(stored, { expanded: false }, theme) as unknown as FakeComponent;
		expect(card.children[0].children.map(text => text.text)).toEqual(["lead → you · demo · direct", "│ body 1"]);
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

	test("a session switch delivers again what the last session was handed but did not consume", async () => {
		for (const reason of ["new", "resume", "fork"]) {
			const stream = liveStream();
			const bus = fakeBus({ streams: [stream.response, () => new Response(streamOf([frame(entry(1))]))] });
			const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
			const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
			await app.emit("session_start");
			await stream.opened;
			stream.push(entry(1));
			await app.emit("session_switch", { type: "session_switch", reason });
			await app.warning("invalid token");
			expect({ reason, sent: app.sent.map(s => s.message.content) }).toEqual({ reason, sent: [envelope(entry(1)), envelope(entry(1))] });
			await app.emit("session_shutdown");
		}
	});

	test("posts held when the session switches do not stall the acks of the next session", async () => {
		const post = entry(1, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "design" });
		const stream = liveStream();
		const bus = fakeBus({ streams: [stream.response, () => new Response(streamOf([frame(entry(2))]))] });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui", idle: false });
		await app.emit("session_start");
		await stream.opened;
		stream.push(post);
		await app.emit("session_switch", { type: "session_switch", reason: "new" });
		await app.warning("invalid token");
		expect(app.sent.map(s => s.message.content)).toEqual([envelope(entry(2))]);
		await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message })));
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 2 }]);
		await app.emit("session_shutdown");
	});

	test("another session holding the stream warns once, retries, and delivers once the stream opens", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const { sent, warnings } = await runSession("tui", {
			streams: [conflict, conflict, () => new Response(streamOf([frame(entry(1))]))],
			tools: { bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }) },
		});
		expect(warnings).toEqual(["agent-bus: another session is already connected as w; inbox not delivered here", "agent-bus: invalid token"]);
		expect(sent.map(s => s.message.content)).toEqual([envelope(entry(1))]);
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

	test("a post held while the session is busy is dropped when the stream stops: nothing reaches omp after logout or shutdown", async () => {
		// "running" is the control: without a stop the held post reaches omp once the window ran out.
		for (const ending of ["running", "logout", "session_shutdown"]) {
			jest.useFakeTimers();
			try {
				const post = entry(1, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "design" });
				const bus = fakeBus({ streams: [() => new Response(streamOf([frame(post)]))] });
				const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
				const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui", idle: false });
				await app.emit("session_start");
				await app.warning("invalid token");
				if (ending === "logout") await app.run("logout");
				if (ending === "session_shutdown") await app.emit(ending);
				jest.advanceTimersByTime(1500);
				expect({ ending, sent: app.sent.length }).toEqual({ ending, sent: ending === "running" ? 1 : 0 });
			} finally {
				jest.useRealTimers();
			}
		}
	});
});

describe("blackboard status line", () => {
	const tempDir = useTempDirs();
	const readThread: RemoteTool = { name: "bus_read_thread", description: "Read", inputSchema: { type: "object", properties: { space: { type: "string" }, threadId: { type: "integer" } } } };
	const post = (id: number) => entry(id, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "design" });

	async function withPosts() {
		const thread = { id: 7, blackboard: "plan", title: "design", createdBy: "lead", subscribed: true, posts: [] };
		const bus = fakeBus({ streams: [() => new Response(streamOf([frame(post(1)), frame(post(2))]))], list: [readThread], tools: { bus_read_thread: () => Response.json({ result: thread }) } });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		await app.warning("invalid token");
		return app;
	}

	test("handed-over posts are counted in the status line, and bus_read_thread clears their thread", async () => {
		const app = await withPosts();
		expect(app.statuses.get("agent-bus")).toBe("bus: 2 new posts in plan/7");
		await app.tool("bus_read_thread").execute("id", { space: "demo", threadId: 7 });
		expect(app.statuses.has("agent-bus")).toBe(false);
		await app.emit("session_shutdown");
	});

	test("/bus logout and a session switch remove the bus entry from the status line", async () => {
		for (const leave of ["logout", "session_switch"]) {
			const app = await withPosts();
			expect(app.statuses.get("agent-bus")).toBe("bus: 2 new posts in plan/7");
			await (leave === "logout" ? app.run("logout") : app.emit(leave, { type: leave, reason: "new" }));
			expect({ leave, status: app.statuses.get("agent-bus") }).toEqual({ leave, status: undefined });
			await app.emit("session_shutdown");
		}
	});
});

describe("context block", () => {
	const tempDir = useTempDirs();
	const tool = (name: string): RemoteTool => ({ name, description: name, inputSchema: { type: "object", properties: { space: { type: "string" } } } });
	const memberWho = { kind: "member", name: "worker", space: "demo", spaces: [{ name: "demo", role: "member" }] };
	const participants = () => Response.json({ result: [{ name: "lead", role: "leader" }, { name: "worker", role: "member" }] });
	const blockOf = (result: unknown) => (result as { messages: { customType?: string; content?: string }[] } | undefined)?.messages[0];

	async function member(options: FakeBusOptions = {}) {
		const bus = fakeBus({ list: [tool("bus_send_direct"), tool("bus_join_space")], tools: { bus_whoami: () => Response.json({ result: memberWho }), bus_list_participants: participants }, ...options });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		return { app, bus };
	}

	test("a connected member gets the block before the request's own messages, with its identity, its leaders and the guide", async () => {
		const { app } = await member();
		const own = { role: "user", content: "hi" };
		const result = (await app.context([own])) as { messages: unknown[] };
		expect(blockOf(result)).toMatchObject({ role: "custom", customType: "agent-bus-context", display: false });
		expect(blockOf(result)?.content).toContain(`You are member \`worker\` on the agent message bus at ${URL_}.\nYour space: \`demo\`. Its leaders: \`lead\`.`);
		expect(blockOf(result)?.content).toContain("\nGUIDE\n");
		expect(result.messages[1]).toBe(own);
		await app.emit("session_shutdown");
	});

	test("a sent message's card names the bus identity the session acts as, the spawn tool's too", async () => {
		const { app } = await member();
		const theme = { fg: (_color: string, text: string) => text, bold: (text: string) => text };
		const title = (name: string, args: unknown) => (app.tool(name).renderCall?.(args, { expanded: false, isPartial: false }, theme) as FakeComponent).children[0].text;
		expect(title("bus_send_direct", { space: "demo", to: "lead", body: "done" })).toBe("worker → lead · demo · direct");
		expect(title("bus_spawn_worker", { space: "demo", name: "w1" })).toBe("worker → w1 · demo · spawn");
		await app.emit("session_shutdown");
	});

	test("other tool calls do not refetch the identity; a membership tool does, and the handler never waits for it", async () => {
		const { promise: slow, resolve: answer } = Promise.withResolvers<Response>();
		const { promise: asked, resolve: ask } = Promise.withResolvers<void>();
		let calls = 0;
		const whoami = () => {
			if (++calls === 1) return Response.json({ result: memberWho });
			ask();
			return slow;
		};
		const { app, bus } = await member({ tools: { bus_whoami: whoami, bus_list_participants: participants } });
		await app.tool("bus_send_direct").execute("id", { space: "demo" });
		expect(callsTo(bus, "/bus_whoami")).toHaveLength(1);
		const joining = app.tool("bus_join_space").execute("id", { space: "demo" });
		await asked;
		expect(blockOf(await app.context([]))?.content).toContain("You are member `worker`");
		answer(Response.json({ result: { ...memberWho, name: "worker2" } }));
		await joining;
		expect(blockOf(await app.context([]))?.content).toContain("You are member `worker2`");
		await app.emit("session_shutdown");
	});

	test("the identity is read before the stream opens, so a turn an inbox entry starts already has the block", async () => {
		const { app, bus } = await member({ streams: [() => new Response(streamOf([frame(entry(1))]))] });
		await app.warning("invalid token");
		const order = bus.calls.map(c => c.url.slice(c.url.lastIndexOf("/") + 1));
		expect(order.indexOf("bus_whoami")).toBeLessThan(order.indexOf("stream"));
		expect(order.indexOf("bus_list_participants")).toBeLessThan(order.indexOf("stream"));
		expect(app.sent).toHaveLength(1);
		expect(blockOf(await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message }))))?.customType).toBe("agent-bus-context");
		await app.emit("session_shutdown");
	});

	test("a server the extension refuses gives no block and is not asked for the identity or the guide", async () => {
		const { app, bus } = await member({ protocol: 1 });
		expect(await app.context([])).toBeUndefined();
		expect([...callsTo(bus, "/bus_whoami"), ...callsTo(bus, "/v1/guide")]).toEqual([]);
		await app.emit("session_shutdown");
	});

	test("promote and a session switch rebuild the block; a token revoked by then drops it", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir() };
		await createCredentialStore(env).save({ url: URL_, adminToken: "adm" });
		let revoked = false;
		const led = [{ name: "demo", role: "leader" }];
		const who = (auth: string | undefined) => (auth === "Bearer lead-tok" ? { kind: "leader", name: "lead", spaces: led } : { kind: "admin", name: "ops", spaces: [] });
		const bus = fakeBus({
			tools: {
				bus_whoami: (_args, auth) => (revoked ? errorResponse(401, "unauthenticated", "invalid token") : Response.json({ result: who(auth) })),
				bus_list_spaces: () => Response.json({ result: [] }),
				bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }),
			},
		});
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" });
		await app.emit("session_start");
		expect(blockOf(await app.context([]))?.content).toContain("You are admin `ops`");
		await app.run("promote lead");
		const promoted = blockOf(await app.context([]))?.content;
		expect(promoted).toContain("You are leader `lead`");
		expect(promoted).not.toContain("You are admin");
		led.push({ name: "beta", role: "leader" });
		await app.emit("session_switch", { type: "session_switch", reason: "new" });
		expect(blockOf(await app.context([]))?.content).toContain("You are leader `lead` on the agent message bus at http://bus.test.\nSpaces you lead: `beta`, `demo`.");
		revoked = true;
		await app.emit("session_switch", { type: "session_switch", reason: "new" });
		expect(await app.context([])).toBeUndefined();
		await app.emit("session_shutdown");
	});

	test("no block without credentials or after logout", async () => {
		const bare = await boot({ fetchFn: fakeBus().fetchFn, env: { OMP_AGENT_BUS_DIR: await tempDir() } }, { mode: "tui" });
		await bare.emit("session_start");
		expect(await bare.context([])).toBeUndefined();
		const { app } = await member();
		await app.run("logout");
		expect(await app.context([])).toBeUndefined();
		await app.emit("session_shutdown");
	});

	test("a guide that cannot be fetched leaves the identity lines and is logged once", async () => {
		const { app } = await member({ guide: () => errorResponse(404, "not_found", "no route") });
		await app.context([]);
		const block = blockOf(await app.context([]))?.content;
		expect(block).toContain("You are member `worker`");
		expect(block).not.toContain("GUIDE");
		expect(app.warnings.filter(w => w.includes("guide"))).toEqual(["agent-bus: guide not loaded: not_found: no route"]);
		await app.emit("session_shutdown");
	});
});

describe("bus events", () => {
	const tempDir = useTempDirs();
	const memberWho = () => Response.json({ result: { kind: "member", name: "worker", space: "demo", spaces: [{ name: "demo", role: "member" }] } });

	/** A member session on `events` whose streams are `streams`; it runs until the streams ran out (the final 401). */
	async function session(events: ReturnType<typeof fakeEvents>, streams: FakeBusOptions["streams"], list: RemoteTool[] = []) {
		const bus = fakeBus({ streams, list, tools: { bus_whoami: memberWho, bus_list_participants: () => Response.json({ result: [] }) } });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env }, { mode: "tui" }, events);
		return { app, bus };
	}

	test("a listener gets every handed-over entry with its delivery mode, and a throwing one stops neither delivery nor ack", async () => {
		const events = fakeEvents();
		const seen: unknown[] = [];
		events.on("agent-bus:entry", data => {
			seen.push(data);
			throw new Error("listener broke");
		});
		const { app, bus } = await session(events, [() => new Response(streamOf([frame(entry(1)), frame(entry(2, { class: "interrupt" }))]))]);
		await app.emit("session_start");
		await app.warning("invalid token");
		expect(seen).toEqual([
			{ entry: entry(1), deliverAs: "aside" },
			{ entry: entry(2, { class: "interrupt" }), deliverAs: "steer" },
		]);
		expect(app.sent).toHaveLength(2);
		await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message })));
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 2 }]);
		await app.emit("session_shutdown");
	});

	test("the connection states are published with the agent and its space", async () => {
		const events = fakeEvents();
		const states: unknown[] = [];
		events.on("agent-bus:connection", data => void states.push(data));
		const conflict = () => errorResponse(409, "conflict", "agent worker already has an active inbox stream");
		const { app } = await session(events, [conflict, conflict, () => new Response(streamOf([frame(entry(1))]))]);
		await app.emit("session_start");
		await app.warning("invalid token");
		await app.run("logout");
		const who = { agent: "worker", space: "demo" };
		expect(states).toEqual([
			{ state: "conflict", ...who },
			{ state: "connected", ...who },
			{ state: "retrying", ...who },
			{ state: "logged-out", ...who },
		]);
		await app.emit("session_shutdown");
	});

	test("of two copies on one event bus only the first registers tools and opens the stream; the second says so", async () => {
		const events = fakeEvents();
		const list = [listSpaces];
		const first = await session(events, [() => new Response(streamOf([]))], list);
		const second = await session(events, [() => new Response(streamOf([]))], list);
		await first.app.emit("session_start");
		await second.app.emit("session_start");
		await first.app.warning("invalid token");
		expect(first.app.tools.map(t => t.name)).toContain("bus_list_spaces");
		expect(second.app.tools).toEqual([]);
		expect(second.app.commands).toEqual([]);
		expect(callsTo(second.bus, "/v1/inbox/stream")).toEqual([]);
		expect(second.app.warnings).toEqual(["agent-bus: another copy of the extension is active in this session; this copy stays off"]);
		await first.app.emit("session_shutdown");
	});
});

describe("bus_wait in omp", () => {
	const tempDir = useTempDirs();
	const serverWait: RemoteTool = {
		name: "bus_wait",
		description: "Wait",
		inputSchema: { type: "object", properties: { timeoutSeconds: { type: "integer" }, from: { type: "string" } } },
	};
	const resultOf = (out: { content: { text: string }[] }): unknown => JSON.parse(out.content[0].text);

	async function bootWorker(options: FakeBusOptions, sleep?: BusExtensionOptions["sleep"]) {
		const bus = fakeBus({ list: [serverWait], ...options });
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const app = await boot({ fetchFn: bus.fetchFn, env, ...(sleep ? { sleep } : {}) }, { mode: "tui" });
		await app.emit("session_start");
		return { bus, app };
	}

	test("a waiting bus_wait takes the matching entry off the stream: omp never gets it, and it counts as consumed", async () => {
		const stream = liveStream();
		const { bus, app } = await bootWorker({ streams: [stream.response] });
		await stream.opened;
		const result = app.tool("bus_wait").execute("id", { from: "lead", timeoutSeconds: 5 });
		stream.push(entry(1, { from: "peer" }));
		stream.push(entry(2, { from: "lead" }));
		expect(resultOf(await result)).toEqual([entry(2, { from: "lead" })]);
		expect(app.sent.map(s => s.message.content)).toEqual([envelope(entry(1, { from: "peer" }))]);
		expect(callsTo(bus, "/bus_inbox_ack")).toEqual([]);
		await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message })));
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 2 }]);
		expect(callsTo(bus, "/v1/tools/bus_wait")).toEqual([]);
		stream.close();
		await app.warning("invalid token");
		await app.emit("session_shutdown");
	});

	test("without its own stream the session forwards bus_wait to the server and acks nothing", async () => {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const { bus, app } = await bootWorker({
			streams: [conflict],
			tools: {
				bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }),
				bus_wait: () => Response.json({ result: [entry(5)] }),
			},
		});
		await app.warning("invalid token");
		expect(resultOf(await app.tool("bus_wait").execute("id", { timeoutSeconds: 10 }))).toEqual([entry(5)]);
		expect(callsTo(bus, "/v1/tools/bus_wait").map(call => call.body)).toEqual([{ timeoutSeconds: 10 }]);
		await app.context([{ role: "toolResult", content: "[]" }]);
		expect(callsTo(bus, "/bus_inbox_ack")).toEqual([]);
		await app.emit("session_shutdown");
	});

	/** Another session holds the stream at first; the loop retries only once `retry` resolves, and then gets `stream`. */
	function takeover(stream: ReturnType<typeof liveStream>, wait: ToolHandler) {
		const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");
		const retry = Promise.withResolvers<void>();
		const options: FakeBusOptions = { streams: [conflict, stream.response], tools: { bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }), bus_wait: wait } };
		return { options, sleep: () => retry.promise, retry: () => retry.resolve() };
	}

	test("an entry a forwarded bus_wait returned is not delivered again when the session's own stream opens; it counts as consumed", async () => {
		const stream = liveStream();
		const { options, sleep, retry } = takeover(stream, () => Response.json({ result: [entry(5)] }));
		const { bus, app } = await bootWorker(options, sleep);
		await app.warning("another session is already connected as w; inbox not delivered here");
		expect(resultOf(await app.tool("bus_wait").execute("id", { timeoutSeconds: 10 }))).toEqual([entry(5)]);
		retry();
		await stream.opened;
		stream.push(entry(5));
		stream.push(entry(6));
		stream.close();
		await app.warning("invalid token");
		expect(app.sent.map(s => s.message.content)).toEqual([envelope(entry(6))]);
		await app.context(app.sent.map(sent => ({ role: "custom", ...sent.message })));
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 5 }, { upTo: 6 }]);
		await app.emit("session_shutdown");
	});

	test("an entry a forwarded bus_wait returned is acked when the session's own stream delivers it, without a model request", async () => {
		const stream = liveStream();
		const { options, sleep, retry } = takeover(stream, () => Response.json({ result: [entry(5)] }));
		const { bus, app } = await bootWorker(options, sleep);
		await app.warning("another session is already connected as w; inbox not delivered here");
		expect(resultOf(await app.tool("bus_wait").execute("id", { timeoutSeconds: 10 }))).toEqual([entry(5)]);
		retry();
		await stream.opened;
		stream.push(entry(5));
		stream.close();
		await app.warning("invalid token");
		expect(app.sent).toEqual([]);
		expect(callsTo(bus, "/bus_inbox_ack").map(call => call.body)).toEqual([{ upTo: 5 }]);
		await app.emit("session_shutdown");
	});

	test("a forwarded bus_wait whose entries the session's own stream already delivered asks again after them", async () => {
		const stream = liveStream();
		const answer = Promise.withResolvers<Response>();
		const answers = [answer.promise, Promise.resolve(Response.json({ result: [entry(6)] }))];
		const { options, sleep, retry } = takeover(stream, () => answers.shift() ?? Response.json({ result: [] }));
		const { bus, app } = await bootWorker(options, sleep);
		await app.warning("another session is already connected as w; inbox not delivered here");
		const waiting = app.tool("bus_wait").execute("id", { timeoutSeconds: 10 });
		retry();
		await stream.opened;
		stream.push(entry(5));
		stream.close();
		await app.warning("invalid token");
		answer.resolve(Response.json({ result: [entry(5)] }));
		expect(resultOf(await waiting)).toEqual([entry(6)]);
		expect(callsTo(bus, "/v1/tools/bus_wait").map(call => call.body)).toEqual([{ timeoutSeconds: 10 }, { after: 5, timeoutSeconds: 10 }]);
		expect(app.sent.map(s => s.message.content)).toEqual([envelope(entry(5))]);
		await app.emit("session_shutdown");
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

	test("every bus tool a main session registers has an explicit tier, and bus_inbox_ack is not among them", async () => {
		const env = { OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "mem" };
		const list = ["bus_send_direct", "bus_inbox_ack", "bus_wait", "bus_delete_space", "bus_something_new"].map(name => ({ ...listSpaces, name }));
		const app = await boot({ fetchFn: fakeBus({ list }).fetchFn, env }, { kind: "main" });
		await app.emit("session_start");
		expect(app.tools.map(t => [t.name, t.approval]).sort()).toEqual([
			["bus_delete_space", "write"],
			["bus_send_direct", "read"],
			["bus_something_new", "write"],
			["bus_spawn_worker", "exec"],
			["bus_wait", "read"],
		]);
		await app.emit("session_shutdown");
	});

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
			"from the stored admin token": async dir => {
				await createCredentialStore({ OMP_AGENT_BUS_DIR: dir }).save({ url: URL_, adminToken: "adm" });
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
			"from the stored admin token": "Bearer adm",
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

	test("the subagent's block says as whom it acts on the bus and holds no guide", async () => {
		const { main, startSub } = await family({ OMP_AGENT_BUS_DIR: await tempDir(), AGENT_BUS_URL: URL_, AGENT_BUS_TOKEN: "env-tok" });
		await main.emit("session_start");
		const sub = await startSub();
		const block = ((await sub.context([])) as { messages: { content: string }[] }).messages[0].content;
		expect(block).toContain("\nYou are a subagent of the main session that acts as leader `lead2` on the agent message bus.\n");
		expect(block).toContain("`[via subagent Explore]`");
		expect(block).not.toContain("GUIDE");
		await main.emit("session_shutdown");
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
		expect((await app.tool("bus_list_spaces").execute("id", {})).content[0].text).toBe(JSON.stringify("Bearer amb_admin_secret", null, 2));
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
