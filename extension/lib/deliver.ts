/** Delivery policy: how an inbox entry enters the session. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { formatEntry } from "./format.ts";
import type { Entry } from "./inbox.ts";

export interface DeliveryOptions {
	deliverAs: "steer" | "followUp";
	triggerTurn: boolean;
}

/**
 * Critical entries steer the running turn, others queue as follow-ups. Only
 * interactive modes (tui, rpc) may start a turn; one-shot CLI prompts (print,
 * json) run their own prompt right after session_start and would fail with
 * AgentBusyError. Unknown mode → no turn.
 */
export function deliveryOptions(entry: Entry, mode: string | undefined): DeliveryOptions {
	return { deliverAs: entry.critical ? "steer" : "followUp", triggerTurn: mode === "tui" || mode === "rpc" };
}

/** Sends each entry as a displayed `agent-bus` message, using the session mode at delivery time. */
export function createDeliver(pi: Pick<ExtensionAPI, "sendMessage">, getMode: () => string | undefined): (entry: Entry) => Promise<void> {
	return async entry => {
		pi.sendMessage({ customType: "agent-bus", content: formatEntry(entry), display: true }, deliveryOptions(entry, getMode()));
	};
}
