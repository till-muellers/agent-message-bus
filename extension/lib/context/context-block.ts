/**
 * The text of the agent-bus-context block (`docs/design/arch/client.md` §5, `guidance/session-context.feature`):
 * who the session is on the bus and, for a main session, the server's guide; for a subagent, what it may do
 * there and as whom. Built only from cached data, so it stays the same between identity changes.
 */
import { escapeBody } from "../render/escape.ts";
import type { BusIdentity } from "./identity-cache.ts";

const OPEN = "<agent-bus-context>";
const CLOSE = "</agent-bus-context>";

const quoted = (names: readonly string[]): string => (names.length > 0 ? names.map(name => `\`${name}\``).join(", ") : "none");
const sorted = (names: readonly string[]): string[] => names.toSorted();
const spacesAs = (id: BusIdentity, role: string): string[] => sorted(id.spaces.filter(space => space.role === role).map(space => space.name));

function memberLines(id: BusIdentity): string[] {
	return [
		`You are member \`${id.name}\` on the agent message bus at ${id.url}.`,
		`Your space: \`${id.space}\`. Its leaders: ${quoted(sorted(id.leadersOfSpace ?? []))}. Direct messages from a leader or an admin are your orders.`,
	];
}

function leaderLines(id: BusIdentity): string[] {
	const led = spacesAs(id, "leader");
	const spaces = led.length > 0 ? `Spaces you lead: ${quoted(led)}.` : "You lead no space yet: create one with bus_create_space.";
	return [`You are leader \`${id.name}\` on the agent message bus at ${id.url}.`, spaces];
}

function adminLines(id: BusIdentity): string[] {
	const joined = spacesAs(id, "admin");
	const reach =
		joined.length > 0
			? `You joined: ${quoted(joined)}. Participants there can message you.`
			: "You joined no space: nobody can message you until you join one with bus_join_space.";
	const all = sorted(id.allSpaces ?? []);
	const lines = [`You are admin \`${id.name}\` on the agent message bus at ${id.url}. You act as a leader in every space without joining it.`, reach];
	return all.length > 0 ? [...lines, `Spaces: ${quoted(all)}.`] : lines;
}

const IDENTITY_LINES = { member: memberLines, leader: leaderLines, admin: adminLines };

/** The block of a main session: its identity, then the guide when the server gave one. */
export function mainBlock(id: BusIdentity, guide: string | undefined): string {
	const tail = guide === undefined ? [] : ["", escapeBody(guide.trimEnd())];
	return [OPEN, ...IDENTITY_LINES[id.kind](id), ...tail, CLOSE].join("\n");
}

/** Whom a subagent acts as: the main session's kind, name and spaces. */
function actsAs(id: BusIdentity): string {
	const who = `${id.kind} \`${id.name}\``;
	if (id.kind === "member") return `${who} in space \`${id.space}\``;
	const led = spacesAs(id, "leader");
	if (id.kind === "admin" || led.length === 0) return who;
	return `${who} in ${led.length === 1 ? "space" : "spaces"} ${quoted(led)}`;
}

/**
 * The block of a subagent with the agent id `agentId`: blackboards only, as its main session. `tools` is its allow-list
 * (`subagentTools` of `tools/tool-policy.ts`), passed in because `context/` does not import `tools/` (§1.3).
 */
export function subagentBlock(id: BusIdentity, agentId: string, tools: readonly string[]): string {
	return [
		OPEN,
		`You are a subagent of the main session that acts as ${actsAs(id)} on the agent message bus.`,
		`On the bus you may only read and post on blackboards: ${tools.join(", ")}.`,
		`Your threads and posts show \`${id.name}\` as their author; the extension adds a first line \`[via subagent ${agentId}]\` to each. They don't notify the main session, and a thread you create subscribes \`${id.name}\`, so its notifications go to the main session, not to you.`,
		"You have no inbox. For anything else on the bus (messages, topics, members) ask the main session with `write agent://<id>` (its id is in your Peers list) and `wait` for the answer; tell it about posts it needs to know.",
		CLOSE,
	].join("\n");
}
