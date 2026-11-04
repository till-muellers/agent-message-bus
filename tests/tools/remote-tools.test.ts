import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createClient, type FetchLike, type RemoteTool } from "../../extension/lib/bus/client.ts";
import type { JsonSchema } from "../../extension/lib/bus/schema.ts";
import { registerRemoteTools, type RemoteToolsOptions } from "../../extension/lib/tools/remote-tools.ts";
import { entry, errorResponse, fakeComponents, type FakeComponent, URL_, z } from "../fakes.ts";

type FakeTool = {
	name: string;
	label: string;
	loadMode?: string;
	approval?: string;
	execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }>;
	renderResult?: (result: unknown, options: { expanded: boolean; isPartial: boolean }, theme: unknown) => unknown;
};
const listTool = (name: string, inputSchema: JsonSchema = { type: "object", properties: {} }): RemoteTool => ({ name, description: name, inputSchema });

function setup(fetchFn: FetchLike, getActive: RemoteToolsOptions["getActive"] = () => ({ url: URL_, token: "t" })) {
	const tools: FakeTool[] = [];
	const warnings: string[] = [];
	const pi = { zod: z, pi: fakeComponents, registerTool: (t: FakeTool) => void tools.push(t) } as unknown as ExtensionAPI;
	const options: RemoteToolsOptions = {
		client: createClient(fetchFn),
		getActive,
		state: { registered: false, names: [] },
		warn: m => void warnings.push(m),
		kind: "main",
	};
	return { tools, warnings, options, register: (overrides: Partial<RemoteToolsOptions> = {}) => registerRemoteTools(pi, { ...options, ...overrides }) };
}

describe("registerRemoteTools", () => {
	test("a successful membership tool refreshes the identity before it returns; other or failed calls do not", async () => {
		const membership = ["bus_create_space", "bus_join_space", "bus_leave_space", "bus_delete_space"];
		let fail = false;
		const fetchFn: FetchLike = async url => {
			if (url.endsWith("/v1/tools")) return Response.json([...membership, "bus_send_direct"].map(name => listTool(name)));
			return fail ? errorResponse(403, "forbidden", "no") : Response.json({ result: { ok: true } });
		};
		const refreshed: string[] = [];
		const { tools, register } = setup(fetchFn);
		let current = "";
		await register({ membershipChanged: async () => void refreshed.push(current) });
		for (const tool of tools) {
			current = tool.name;
			await tool.execute("id", { space: "demo" });
		}
		expect(refreshed).toEqual(membership);
		fail = true;
		await expect(tools[0].execute("id", { space: "demo" })).rejects.toThrow("forbidden: no");
		expect(refreshed).toEqual(membership);
	});

	test("bus_read_thread renders its result as a thread, and a successful read reports the thread as read", async () => {
		const thread = { id: 7, blackboard: "plan", title: "design", createdBy: "peer", subscribed: true, posts: [] };
		let fail = false;
		const fetchFn: FetchLike = async url => {
			if (url.endsWith("/v1/tools")) return Response.json([listTool("bus_read_thread"), listTool("bus_a")]);
			return fail ? errorResponse(404, "not_found", "no thread") : Response.json({ result: thread });
		};
		const reads: [string, number][] = [];
		const { tools, register } = setup(fetchFn);
		await register({ threadRead: (space, threadId) => void reads.push([space, threadId]) });
		const [read, other] = tools;
		expect(other.renderResult).toBeUndefined();
		const out = await read.execute("id", { space: "demo", threadId: 7 });
		expect(reads).toEqual([["demo", 7]]);
		const theme = { fg: (_color: string, text: string) => text, bold: (text: string) => text };
		const view = read.renderResult?.(out, { expanded: false, isPartial: false }, theme) as FakeComponent;
		expect(view.children[0].text).toBe("design plan / 7 · 0 posts");
		fail = true;
		await expect(read.execute("id", { space: "demo", threadId: 7 })).rejects.toThrow("not_found: no thread");
		expect(reads).toEqual([["demo", 7]]);
	});

	test("registers once and executes with the current credentials", async () => {
		const seen: (string | undefined)[] = [];
		const fetchFn: FetchLike = async (url, init) => {
			if (url.endsWith("/v1/tools"))
				return Response.json([{ name: "bus_list_spaces", description: "List", inputSchema: { type: "object", properties: {}, additionalProperties: false } }]);
			seen.push(init.headers.Authorization);
			return Response.json({ result: [{ name: "demo" }] });
		};
		let token: string | undefined = "one";
		const { tools, register } = setup(fetchFn, () => (token ? { url: URL_, token } : undefined));
		expect(await register()).toBe(1);
		expect(await register()).toBe(0);
		expect(tools.map(t => [t.name, t.label])).toEqual([["bus_list_spaces", "bus_list_spaces"]]);
		token = undefined;
		await expect(tools[0].execute("id", {})).rejects.toThrow("Not logged in to the agent bus. Run /bus login <url> <token>.");
		token = "two";
		const out = await tools[0].execute("id", {});
		expect(out.content[0].text).toBe(JSON.stringify([{ name: "demo" }], null, 2));
		expect(seen).toEqual(["Bearer two"]);
	});

	test("a bus error from a tool call is thrown as <code>: <message>", async () => {
		const fetchFn: FetchLike = async url =>
			url.endsWith("/v1/tools") ? Response.json([listTool("bus_a")]) : errorResponse(403, "forbidden", "only leaders may invite");
		const { tools, register } = setup(fetchFn);
		await register();
		await expect(tools[0].execute("id", {})).rejects.toThrow("forbidden: only leaders may invite");
	});

	test("a tool that fails to register is reported; the others still register", async () => {
		const list = [listTool("bus_a"), listTool("bus_bad", { type: "object", properties: { xs: { type: "array" } } }), listTool("bus_c")];
		const { tools, warnings, register } = setup(async () => Response.json(list));
		expect(await register()).toBe(2);
		expect(tools.map(t => t.name)).toEqual(["bus_a", "bus_c"]);
		expect(warnings).toEqual(["tool bus_bad not registered: unsupported schema type array"]);
	});

	test("a failed list fetch leaves registration open for the next attempt", async () => {
		let fail = true;
		const { tools, options, register } = setup(async () => (fail ? errorResponse(500, "internal", "boom") : Response.json([listTool("bus_a")])));
		await expect(register()).rejects.toThrow("boom");
		expect(options.state.registered).toBe(false);
		fail = false;
		expect(await register()).toBe(1);
		expect(options.state.registered).toBe(true);
		expect(tools.map(t => t.name)).toEqual(["bus_a"]);
	});

	test("a main session registers its own bus_wait instead of the remote one and withholds bus_inbox_ack; the names are recorded", async () => {
		const list = [listTool("bus_inbox_read"), listTool("bus_send"), listTool("bus_wait"), listTool("bus_inbox_ack")];
		const urls: string[] = [];
		const { tools, options, register } = setup(async url => {
			urls.push(url);
			return Response.json(list);
		});
		const wait = { dispatcher: () => ({ takeHeld: () => [], waitNext: async () => [entry(1)] }), forward: async () => [], consumed: () => {} };
		expect(await register({ wait })).toBe(3);
		expect(options.state.names).toEqual(["bus_inbox_read", "bus_send", "bus_wait"]);
		const out = await tools[2].execute("id", {});
		expect(JSON.parse(out.content[0].text)).toEqual([entry(1)]);
		expect(urls).toEqual([`${URL_}/v1/tools`]);
	});

	test("a subagent gets exactly the allow-list", async () => {
		const list = [
			listTool("bus_list_blackboards"),
			listTool("bus_read_thread"),
			listTool("bus_add_post"),
			listTool("bus_send_direct"),
			listTool("bus_wait"),
			listTool("bus_something_new"),
		];
		const { options, register } = setup(async () => Response.json(list));
		expect(await register({ kind: "sub" })).toBe(3);
		expect(options.state.names).toEqual(["bus_list_blackboards", "bus_read_thread", "bus_add_post"]);
	});

	test("every registered tool carries its load mode and approval tier, the local bus_wait included", async () => {
		const list = [listTool("bus_send_direct"), listTool("bus_delete_space"), listTool("bus_wait"), listTool("bus_something_new")];
		const { tools, register } = setup(async () => Response.json(list));
		const wait = { dispatcher: () => undefined, forward: async () => [], consumed: () => {} };
		await register({ wait });
		expect(tools.map(t => [t.name, t.loadMode, t.approval])).toEqual([
			["bus_send_direct", "essential", "read"],
			["bus_delete_space", "discoverable", "write"],
			["bus_something_new", "discoverable", "write"],
			["bus_wait", "essential", "read"],
		]);
	});

	test("the mail hint a result carries never reaches the model", async () => {
		const fetchFn: FetchLike = async url =>
			url.endsWith("/v1/tools") ? Response.json([listTool("bus_whoami")]) : Response.json({ result: { name: "w" }, mail: { waiting: 2, critical: 0, spaces: ["demo"] } });
		const { tools, register } = setup(fetchFn);
		await register();
		expect((await tools[0].execute("id", {})).content[0].text).toBe(JSON.stringify({ name: "w" }, null, 2));
	});
});
