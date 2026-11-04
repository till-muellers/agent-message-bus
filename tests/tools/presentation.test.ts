import { describe, expect, test } from "bun:test";
import { presentationOf, TOOL_PRESENTATION } from "../../extension/lib/tools/presentation.ts";

/** `omp-tools.feature` rules 1-2: every server tool but bus_inbox_ack (withheld), plus bus_spawn_worker. */
const EXPECTED: [string, "essential" | "discoverable", "read" | "write" | "exec"][] = [
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

describe("tool presentation", () => {
	test("every bus tool has its load mode and approval tier", () => {
		for (const [name, loadMode, approval] of EXPECTED) expect({ name, ...presentationOf(name) }).toEqual({ name, loadMode, approval });
	});

	test("the table has a row for exactly these tools: no bus_cancel_invite (PO-3) and no bus_inbox_ack (withheld)", () => {
		expect(Object.keys(TOOL_PRESENTATION).sort()).toEqual(EXPECTED.map(([name]) => name).sort());
	});

	test("a tool the server adds later is discoverable and needs write approval", () => {
		expect(presentationOf("bus_something_new")).toEqual({ loadMode: "discoverable", approval: "write" });
	});
});
