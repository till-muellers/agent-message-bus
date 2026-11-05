/**
 * The extension's policy per bus tool, one row per tool it knows (`docs/design/arch/client.md` §5-7, `omp-tools.feature`
 * rules 1-2, PO-2): how omp presents it, whether a subagent gets it, whether omp's own local tool shadows it, whether a
 * subagent's body is attributed, whether it changes the session's spaces, whether it sends a message and whether it
 * reads a thread. The messaging and blackboard core are top-level (`essential`), the rest is found through tool search;
 * reads, sends, posts and thread creation need `read` approval, management `write`, starting a worker `exec`.
 */
import type { RemoteTool } from "../bus/client.ts";
import type { SendKind } from "../render/outgoing-card.ts";

/** omp's `ToolLoadMode` and the decision values of its `ToolApproval` (`pi-agent-core` types). */
export interface Presentation {
	loadMode: "essential" | "discoverable";
	approval: "read" | "write" | "exec";
}

export interface ToolPolicy extends Presentation {
	/** On the subagent allow-list (§6.1): a tool the server adds later stays main-only. */
	subagent?: true;
	/** omp provides it itself in a main session: the extension's local tool replaces the server's. */
	shadowed?: true;
	/** In a subagent, the body carries the attribution line of the subagent that wrote it (§6.3). */
	attributed?: true;
	/** A successful call changes the session's spaces: the identity is read again before the tool returns (§5). */
	refreshesMembership?: true;
	/** It sends a message: its call renders as an outgoing card, its result as a delivery receipt of this kind. */
	send?: SendKind;
	/** It returns one thread: its result renders as the thread view, and a successful call marks the thread read. */
	readsThread?: true;
}

/** A server tool the extension never registers: it owns the acknowledgement, which follows the consumption. */
type Withheld = "withheld";

type Tier = Presentation["approval"];
type Flags = Omit<ToolPolicy, keyof Presentation>;
const essential = (approval: Tier, flags: Flags = {}): ToolPolicy => ({ loadMode: "essential", approval, ...flags });
const discoverable = (approval: Tier, flags: Flags = {}): ToolPolicy => ({ loadMode: "discoverable", approval, ...flags });

/** Every server tool and every local tool of the extension. */
export const TOOL_POLICY: Readonly<Record<string, ToolPolicy | Withheld>> = {
	bus_send_direct: essential("read", { send: "direct" }),
	bus_send_topic: essential("read", { send: "topic" }),
	bus_wait: essential("read", { shadowed: true }),
	// The blackboard tools, a subagent's allow-list, in the order the subagent's context block names them.
	bus_list_blackboards: discoverable("read", { subagent: true }),
	bus_list_threads: discoverable("read", { subagent: true }),
	bus_read_thread: essential("read", { subagent: true, readsThread: true }),
	bus_create_thread: discoverable("read", { subagent: true, attributed: true }),
	bus_add_post: essential("read", { subagent: true, attributed: true }),
	bus_whoami: discoverable("read"),
	bus_list_spaces: discoverable("read"),
	bus_list_participants: discoverable("read"),
	bus_list_topics: discoverable("read"),
	bus_inbox_read: discoverable("read"),
	bus_inbox_ack: "withheld",
	bus_list_admins: discoverable("read"),
	bus_space_overview: discoverable("read"),
	bus_create_topic: discoverable("write"),
	bus_subscribe_topic: discoverable("write"),
	bus_unsubscribe_topic: discoverable("write"),
	bus_close_topic: discoverable("write"),
	bus_create_blackboard: discoverable("write"),
	bus_delete_blackboard: discoverable("write"),
	bus_remove_thread: discoverable("write"),
	bus_subscribe_thread: discoverable("write"),
	bus_unsubscribe_thread: discoverable("write"),
	bus_create_space: discoverable("write", { refreshesMembership: true }),
	bus_join_space: discoverable("write", { refreshesMembership: true }),
	bus_leave_space: discoverable("write", { refreshesMembership: true }),
	bus_delete_space: discoverable("write", { refreshesMembership: true }),
	bus_invite_member: discoverable("write"),
	bus_revoke_member: discoverable("write"),
	bus_mint_leader: discoverable("write"),
	bus_mint_admin: discoverable("write"),
	bus_revoke_admin: discoverable("write"),
	bus_spawn_worker: discoverable("exec"),
};

/** A tool the server adds later is found through search, asks before it writes and carries no flag. */
const UNKNOWN = discoverable("write");

/** The policy of a tool the extension registers; the default one for an unknown tool. */
export function policyOf(name: string): ToolPolicy {
	const row = TOOL_POLICY[name];
	return row === undefined || row === "withheld" ? UNKNOWN : row;
}

/** How omp presents the tool: the two fields of its tool definition. */
export function presentationOf(name: string): Presentation {
	const { loadMode, approval } = policyOf(name);
	return { loadMode, approval };
}

/** The names of the subagent's allow-list in table order, for the subagent's context block (§6.1). */
export function subagentTools(): string[] {
	return Object.keys(TOOL_POLICY).filter(name => policyOf(name).subagent);
}

/**
 * The server tools a session of this kind registers (§6.1): a subagent the allow-list, a main session everything but
 * the shadowed and the withheld tools. The withholding happens at registration, not at activation: omp enables every
 * tool an extension registers unless it is `defaultInactive` or hidden.
 */
export function selectTools(remote: readonly RemoteTool[], kind: "main" | "sub"): RemoteTool[] {
	if (kind === "sub") return remote.filter(tool => policyOf(tool.name).subagent);
	return remote.filter(tool => TOOL_POLICY[tool.name] !== "withheld" && !policyOf(tool.name).shadowed);
}
