import { describe, expect, test } from "bun:test";
import type { RemoteTool } from "../../extension/lib/bus/client.ts";
import { selectTools, SHADOWED_TOOLS, SUBAGENT_TOOLS, WITHHELD_TOOLS } from "../../extension/lib/tools/tool-selection.ts";

const tool = (name: string): RemoteTool => ({ name, description: name, inputSchema: { type: "object", properties: {} } });

const SERVER_TOOLS = [
	"bus_send_direct",
	"bus_list_blackboards",
	"bus_list_threads",
	"bus_read_thread",
	"bus_create_thread",
	"bus_add_post",
	"bus_inbox_read",
	"bus_inbox_ack",
	"bus_wait",
];

describe("selectTools", () => {
	test("a main session gets every server tool but the shadowed and the withheld ones", () => {
		const remote = SERVER_TOOLS.map(tool);
		const names = selectTools(remote, "main").map(t => t.name);
		expect(names).toEqual(["bus_send_direct", "bus_list_blackboards", "bus_list_threads", "bus_read_thread", "bus_create_thread", "bus_add_post", "bus_inbox_read"]);
		expect(names).not.toContain("bus_wait");
		expect(names).not.toContain("bus_inbox_ack");
	});

	test("a subagent gets exactly the five blackboard tools", () => {
		const remote = SERVER_TOOLS.map(tool);
		expect(selectTools(remote, "sub").map(t => t.name)).toEqual([
			"bus_list_blackboards",
			"bus_list_threads",
			"bus_read_thread",
			"bus_create_thread",
			"bus_add_post",
		]);
	});

	test("a tool the server adds later is withheld from a subagent", () => {
		const remote = [...SERVER_TOOLS, "bus_something_new"].map(tool);
		expect(selectTools(remote, "sub").map(t => t.name)).not.toContain("bus_something_new");
		expect(selectTools(remote, "main").map(t => t.name)).toContain("bus_something_new");
	});

	test("a tool of the allow-list that the server does not offer is not registered", () => {
		expect(selectTools([tool("bus_send_direct")], "sub")).toEqual([]);
	});

	test("the allow-list, the shadowed and the withheld tools are the ones of the design", () => {
		expect(SUBAGENT_TOOLS).toEqual(["bus_list_blackboards", "bus_list_threads", "bus_read_thread", "bus_create_thread", "bus_add_post"]);
		expect(SHADOWED_TOOLS).toEqual(["bus_wait"]);
		expect(WITHHELD_TOOLS).toEqual(["bus_inbox_ack"]);
	});
});
