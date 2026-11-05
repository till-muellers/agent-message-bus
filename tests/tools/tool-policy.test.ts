/**
 * The tool policy table (`docs/design/arch/client.md` §6-7, `omp-tools.feature` rules 1-2, `omp-subagents.feature`):
 * one row per tool the extension knows, checked against the server registry as `server/TOOLS.md` renders it.
 */
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { RemoteTool } from "../../extension/lib/bus/client.ts";
import { SPAWN_TOOL } from "../../extension/lib/spawn/spawn-tool.ts";
import { policyOf, presentationOf, selectTools, subagentTools, TOOL_POLICY, type ToolPolicy } from "../../extension/lib/tools/tool-policy.ts";

/** The server registry: the index of the generated ledger, which a Go golden test keeps in sync with it. */
const LEDGER = resolve(import.meta.dir, "..", "..", "server", "TOOLS.md");
const SERVER_TOOLS = [...readFileSync(LEDGER, "utf8").matchAll(/^\| \[(bus_\w+)\]\(#/gm)].map(match => match[1] ?? "");

type Tier = ToolPolicy["approval"];

/** `omp-tools.feature` rules 1-2: every server tool but bus_inbox_ack (withheld), plus bus_spawn_worker. */
const PRESENTATION: [string, ToolPolicy["loadMode"], Tier][] = [
	["bus_send_direct", "essential", "read"],
	["bus_send_topic", "essential", "read"],
	["bus_wait", "essential", "read"],
	["bus_read_thread", "essential", "read"],
	["bus_add_post", "essential", "read"],
	["bus_whoami", "discoverable", "read"],
	["bus_list_spaces", "discoverable", "read"],
	["bus_list_participants", "discoverable", "read"],
	["bus_list_topics", "discoverable", "read"],
	["bus_create_topic", "discoverable", "write"],
	["bus_subscribe_topic", "discoverable", "write"],
	["bus_unsubscribe_topic", "discoverable", "write"],
	["bus_close_topic", "discoverable", "write"],
	["bus_list_blackboards", "discoverable", "read"],
	["bus_list_threads", "discoverable", "read"],
	["bus_create_thread", "discoverable", "read"],
	["bus_create_blackboard", "discoverable", "write"],
	["bus_delete_blackboard", "discoverable", "write"],
	["bus_remove_thread", "discoverable", "write"],
	["bus_subscribe_thread", "discoverable", "write"],
	["bus_unsubscribe_thread", "discoverable", "write"],
	["bus_inbox_read", "discoverable", "read"],
	["bus_create_space", "discoverable", "write"],
	["bus_join_space", "discoverable", "write"],
	["bus_leave_space", "discoverable", "write"],
	["bus_delete_space", "discoverable", "write"],
	["bus_invite_member", "discoverable", "write"],
	["bus_revoke_member", "discoverable", "write"],
	["bus_mint_leader", "discoverable", "write"],
	["bus_mint_admin", "discoverable", "write"],
	["bus_list_admins", "discoverable", "read"],
	["bus_revoke_admin", "discoverable", "write"],
	["bus_space_overview", "discoverable", "read"],
	["bus_spawn_worker", "discoverable", "exec"],
];

type Flag = "subagent" | "shadowed" | "attributed" | "refreshesMembership" | "readsThread";

/** The tools that carry each flag (`client.md` §5, §6.1, §6.3). */
const FLAGGED: Record<Flag, string[]> = {
	subagent: ["bus_list_blackboards", "bus_list_threads", "bus_read_thread", "bus_create_thread", "bus_add_post"],
	shadowed: ["bus_wait"],
	attributed: ["bus_add_post", "bus_create_thread"],
	refreshesMembership: ["bus_create_space", "bus_join_space", "bus_leave_space", "bus_delete_space"],
	readsThread: ["bus_read_thread"],
};

const rows = (): [string, ToolPolicy][] => Object.keys(TOOL_POLICY).flatMap(name => (TOOL_POLICY[name] === "withheld" ? [] : [[name, policyOf(name)]]));
const tool = (name: string): RemoteTool => ({ name, description: name, inputSchema: { type: "object", properties: {} } });
const names = (tools: RemoteTool[]): string[] => tools.map(t => t.name);

describe("the tool policy table", () => {
	test("every server tool and every local tool has a row, and no row is without a tool", () => {
		expect(SERVER_TOOLS).toContain("bus_send_direct");
		expect(Object.keys(TOOL_POLICY).sort()).toEqual([...SERVER_TOOLS, SPAWN_TOOL].sort());
	});

	test("every registered bus tool has its load mode and approval tier", () => {
		for (const [name, loadMode, approval] of PRESENTATION) expect({ name, ...presentationOf(name) }).toEqual({ name, loadMode, approval });
		expect(rows().map(([name]) => name).sort()).toEqual(PRESENTATION.map(([name]) => name).sort());
	});

	test("the extension withholds bus_inbox_ack: it owns the acknowledgement", () => {
		expect(Object.keys(TOOL_POLICY).filter(name => TOOL_POLICY[name] === "withheld")).toEqual(["bus_inbox_ack"]);
	});

	test("each flag sits on exactly the tools of the design", () => {
		for (const flag of Object.keys(FLAGGED) as Flag[]) {
			const tools = rows().flatMap(([name, row]) => (row[flag] ? [name] : []));
			expect({ flag, tools: tools.sort() }).toEqual({ flag, tools: [...FLAGGED[flag]].sort() });
		}
	});

	test("the send tools render an outgoing card and a receipt of their kind", () => {
		expect(rows().flatMap(([name, row]) => (row.send ? [[name, row.send]] : []))).toEqual([
			["bus_send_direct", "direct"],
			["bus_send_topic", "topic"],
		]);
	});

	test("a tool the server adds later is discoverable, needs write approval and carries no flag", () => {
		expect(policyOf("bus_something_new")).toEqual({ loadMode: "discoverable", approval: "write" });
		expect(presentationOf("bus_something_new")).toEqual({ loadMode: "discoverable", approval: "write" });
	});

	test("the presentation of a tool is only its load mode and approval", () => {
		expect(presentationOf("bus_add_post")).toEqual({ loadMode: "essential", approval: "read" });
	});

	test("the subagent tools are named in the order the subagent's block lists them", () => {
		expect(subagentTools()).toEqual(["bus_list_blackboards", "bus_list_threads", "bus_read_thread", "bus_create_thread", "bus_add_post"]);
	});
});

describe("selectTools", () => {
	test("a main session gets every server tool but the shadowed and the withheld ones", () => {
		const selected = names(selectTools(SERVER_TOOLS.map(tool), "main"));
		expect(selected.sort()).toEqual(SERVER_TOOLS.filter(name => name !== "bus_wait" && name !== "bus_inbox_ack").sort());
	});

	test("a subagent gets exactly the five blackboard tools", () => {
		expect(names(selectTools(SERVER_TOOLS.map(tool), "sub")).sort()).toEqual([...FLAGGED.subagent].sort());
	});

	test("a tool the server adds later stays with the main session", () => {
		const remote = [...SERVER_TOOLS, "bus_something_new"].map(tool);
		expect(names(selectTools(remote, "sub"))).not.toContain("bus_something_new");
		expect(names(selectTools(remote, "main"))).toContain("bus_something_new");
	});

	test("a tool of the allow-list that the server does not offer is not registered", () => {
		expect(selectTools([tool("bus_send_direct")], "sub")).toEqual([]);
	});
});
