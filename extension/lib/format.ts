/** Renders an inbox entry as the `[agent-bus]` session message text. */
import type { Entry } from "./inbox.ts";

function where(e: Entry): string {
	switch (e.type) {
		case "direct_message":
			return "direct";
		case "topic_message":
			return `topic #${e.topic}`;
		case "topic_closed":
			return `topic #${e.topic} closed`;
		case "thread_post":
			return `thread ${e.threadId} "${e.threadTitle}" on ${e.blackboard}`;
		case "thread_removed":
			return `thread ${e.threadId} "${e.threadTitle}" removed`;
		case "blackboard_removed":
			return `blackboard ${e.blackboard} removed`;
	}
}

export function formatEntry(e: Entry): string {
	return `[agent-bus] ${e.space} · ${where(e)} · from ${e.from}${e.critical ? " · CRITICAL" : ""}\n${e.body ?? ""}`;
}
