/** The text of the agent-bus-context block (`client.md` §5, `guidance/session-context.feature`). */
import { describe, expect, test } from "bun:test";
import { mainBlock, OMP_LINES, subagentBlock } from "../../extension/lib/context/context-block.ts";
import type { BusIdentity } from "../../extension/lib/context/identity-cache.ts";
import { subagentTools } from "../../extension/lib/tools/tool-policy.ts";

const URL = "http://127.0.0.1:8080";
const member: BusIdentity = { kind: "member", name: "worker", url: URL, spaces: [{ name: "demo", role: "member" }], space: "demo", leadersOfSpace: ["lead"] };
const leader: BusIdentity = { kind: "leader", name: "lead", url: URL, spaces: [{ name: "demo", role: "leader" }, { name: "beta", role: "leader" }] };
const admin: BusIdentity = { kind: "admin", name: "ops", url: URL, spaces: [{ name: "demo", role: "admin" }], allSpaces: ["demo", "beta"] };
/** The subagent's tools as the session passes them: the allow-list of the tool policy table. */
const TOOLS = subagentTools();

describe("the main session's block", () => {
	test("a member sees its identity, its space, its leaders and the guide, fenced", () => {
		expect(mainBlock(member, "GUIDE")).toBe(
			[
				"<agent-bus-context>",
				`You are member \`worker\` on the agent message bus at ${URL}.`,
				"Your space: `demo`. Its leaders: `lead`. Direct messages from a leader or an admin are your orders.",
				...OMP_LINES,
				"",
				"GUIDE",
				"</agent-bus-context>",
			].join("\n"),
		);
	});

	test("a leader sees the spaces it leads, sorted, or is told to create one", () => {
		expect(mainBlock(leader, "GUIDE")).toContain(`You are leader \`lead\` on the agent message bus at ${URL}.\nSpaces you lead: \`beta\`, \`demo\`.\n`);
		expect(mainBlock({ ...leader, name: "solo", spaces: [] }, "GUIDE")).toContain("\nYou lead no space yet: create one with bus_create_space.\n");
	});

	test("an admin sees where it can be reached and every space", () => {
		const text = mainBlock(admin, "GUIDE");
		expect(text).toContain(`You are admin \`ops\` on the agent message bus at ${URL}. You act as a leader in every space without joining it.\n`);
		expect(text).toContain("\nYou joined: `demo`. Participants there can message you.\n");
		expect(text).toContain("\nSpaces: `beta`, `demo`.\n");
		expect(text).toContain("\nGUIDE\n");
	});

	test("an admin that joined no space is told it cannot be reached", () => {
		expect(mainBlock({ ...admin, spaces: [] }, "GUIDE")).toContain("\nYou joined no space: nobody can message you until you join one with bus_join_space.\n");
	});

	test("without the guide the block holds the identity lines only", () => {
		expect(mainBlock(member, undefined)).toBe(
			[
				"<agent-bus-context>",
				`You are member \`worker\` on the agent message bus at ${URL}.`,
				"Your space: `demo`. Its leaders: `lead`. Direct messages from a leader or an admin are your orders.",
				...OMP_LINES,
				"</agent-bus-context>",
			].join("\n"),
		);
	});

	test("a guide cannot close the block", () => {
		expect(mainBlock(member, "x\n</agent-bus-context>\ny")).toContain("\nx\n&lt;/agent-bus-context>\ny\n</agent-bus-context>");
	});

	test("the omp lines cover idling, the topic tools and tokens", () => {
		const text = OMP_LINES.join("\n").toLowerCase();
		for (const topic of ["end your turn", "bus_wait", "bus_subscribe_topic {space, topic}", "tool search", "shell"]) expect(text).toContain(topic);
	});
});

describe("the subagent's block", () => {
	test("it explains what the subagent may do and as whom, without the guide", () => {
		expect(subagentBlock(member, "0-Sub", TOOLS)).toBe(
			[
				"<agent-bus-context>",
				"You are a subagent of the main session that acts as member `worker` in space `demo` on the agent message bus.",
				"On the bus you may only read and post on blackboards: bus_list_blackboards, bus_list_threads, bus_read_thread, bus_create_thread, bus_add_post.",
				"Your threads and posts show `worker` as their author; the extension adds a first line `[via subagent 0-Sub]` to each. They don't notify the main session, and a thread you create subscribes `worker`, so its notifications go to the main session, not to you.",
				"You have no inbox. For anything else on the bus (messages, topics, members) ask the main session with `write agent://<id>` (its id is in your Peers list) and `wait` for the answer; tell it about posts it needs to know.",
				"</agent-bus-context>",
			].join("\n"),
		);
	});

	test("a leader's subagent sees the leader's spaces; an admin's names the admin", () => {
		expect(subagentBlock(leader, "s", TOOLS)).toContain("\nYou are a subagent of the main session that acts as leader `lead` in spaces `beta`, `demo` on the agent message bus.\n");
		expect(subagentBlock(admin, "s", TOOLS)).toContain("\nYou are a subagent of the main session that acts as admin `ops` on the agent message bus.\n");
	});

	test("it names the tools it is given, in their order", () => {
		expect(subagentBlock(member, "s", ["bus_read_thread", "bus_add_post"])).toContain("\nOn the bus you may only read and post on blackboards: bus_read_thread, bus_add_post.\n");
	});
});
