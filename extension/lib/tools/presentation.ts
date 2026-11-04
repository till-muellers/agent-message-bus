/**
 * How omp presents each bus tool (`docs/design/arch/client.md` §7, `omp-tools.feature` rules 1-2, PO-2):
 * the messaging and blackboard core are top-level (`essential`), the rest is found through tool search; reads,
 * sends, posts and thread creation need `read` approval, management `write`, starting a worker `exec`.
 */

/** omp's `ToolLoadMode` and the decision values of its `ToolApproval` (`pi-agent-core` types). */
export interface Presentation {
	loadMode: "essential" | "discoverable";
	approval: "read" | "write" | "exec";
}

type Tier = Presentation["approval"];
const essential = (approval: Tier): Presentation => ({ loadMode: "essential", approval });
const discoverable = (approval: Tier): Presentation => ({ loadMode: "discoverable", approval });

/** Every bus tool the extension knows. `bus_inbox_ack` has no row: the extension withholds it. */
export const TOOL_PRESENTATION: Readonly<Record<string, Presentation>> = {
	bus_send_direct: essential("read"),
	bus_send_topic: essential("read"),
	bus_wait: essential("read"),
	bus_read_thread: essential("read"),
	bus_add_post: essential("read"),
	bus_whoami: discoverable("read"),
	bus_list_spaces: discoverable("read"),
	bus_list_participants: discoverable("read"),
	bus_list_topics: discoverable("read"),
	bus_list_blackboards: discoverable("read"),
	bus_list_threads: discoverable("read"),
	bus_create_thread: discoverable("read"),
	bus_inbox_read: discoverable("read"),
	bus_space_overview: discoverable("read"),
	bus_list_admins: discoverable("read"),
	bus_create_topic: discoverable("write"),
	bus_subscribe_topic: discoverable("write"),
	bus_unsubscribe_topic: discoverable("write"),
	bus_close_topic: discoverable("write"),
	bus_create_blackboard: discoverable("write"),
	bus_delete_blackboard: discoverable("write"),
	bus_remove_thread: discoverable("write"),
	bus_subscribe_thread: discoverable("write"),
	bus_unsubscribe_thread: discoverable("write"),
	bus_create_space: discoverable("write"),
	bus_join_space: discoverable("write"),
	bus_leave_space: discoverable("write"),
	bus_delete_space: discoverable("write"),
	bus_invite_member: discoverable("write"),
	bus_revoke_member: discoverable("write"),
	bus_mint_leader: discoverable("write"),
	bus_mint_admin: discoverable("write"),
	bus_revoke_admin: discoverable("write"),
	bus_spawn_worker: discoverable("exec"),
};

/** A tool the server adds later is found through search and asks before it writes. */
const UNKNOWN = discoverable("write");

export function presentationOf(toolName: string): Presentation {
	return TOOL_PRESENTATION[toolName] ?? UNKNOWN;
}
