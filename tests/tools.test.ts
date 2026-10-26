import { describe, expect, test } from "bun:test";
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { createClient, type FetchLike, type RemoteTool } from "../extension/lib/bus/client.ts";
import type { JsonSchema } from "../extension/lib/bus/schema.ts";
import { INBOX_TOOLS, registerRemoteTools, type RemoteToolsOptions } from "../extension/lib/tools/remote-tools.ts";
import { errorResponse, URL_, z } from "./fakes.ts";

type FakeTool = { name: string; label: string; execute: (...args: unknown[]) => Promise<{ content: { text: string }[] }> };
const listTool = (name: string, inputSchema: JsonSchema = { type: "object", properties: {} }): RemoteTool => ({ name, description: name, inputSchema });

function setup(fetchFn: FetchLike, getActive: RemoteToolsOptions["getActive"] = () => ({ url: URL_, token: "t" })) {
	const tools: FakeTool[] = [];
	const warnings: string[] = [];
	const pi = { zod: z, registerTool: (t: FakeTool) => void tools.push(t) } as unknown as ExtensionAPI;
	const options: RemoteToolsOptions = {
		client: createClient(fetchFn),
		getActive,
		state: { registered: false, names: [] },
		warn: m => void warnings.push(m),
	};
	return { tools, warnings, options, register: (exclude?: readonly string[]) => registerRemoteTools(pi, { ...options, exclude }) };
}

describe("registerRemoteTools", () => {
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

	test("excluded tools are not registered; registered names are recorded", async () => {
		const list = [listTool("bus_inbox_read"), listTool("bus_send"), listTool("bus_inbox_ack")];
		const { options, register } = setup(async () => Response.json(list));
		expect(await register(INBOX_TOOLS)).toBe(1);
		expect(options.state.names).toEqual(["bus_send"]);
	});
});
