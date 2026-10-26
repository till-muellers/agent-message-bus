/**
 * How an inbox entry enters the session: the only place that knows omp's delivery mechanisms.
 * The server's delivery class and the phase of the session decide the mechanism
 * (`docs/design/arch/client.md` §3.2).
 */
import type { DeliveryClass } from "../bus/entry.ts";

/** What the session is doing: interactive sessions may steer and start turns, one-shot sessions may not. */
export type SessionPhase = "interactive-busy" | "interactive-idle" | "oneshot-busy" | "oneshot-idle";

/** How omp is told to deliver a message. */
export interface Mechanism {
	deliverAs: "steer" | "aside" | "nextTurn";
	triggerTurn: boolean;
}

/** The mechanism of every class in every phase. */
const MECHANISMS: Record<DeliveryClass, Record<SessionPhase, Mechanism>> = {
	interrupt: {
		"interactive-busy": { deliverAs: "steer", triggerTurn: false },
		"interactive-idle": { deliverAs: "steer", triggerTurn: true },
		"oneshot-busy": { deliverAs: "steer", triggerTurn: false },
		"oneshot-idle": { deliverAs: "nextTurn", triggerTurn: false },
	},
	queue: {
		"interactive-busy": { deliverAs: "aside", triggerTurn: false },
		"interactive-idle": { deliverAs: "aside", triggerTurn: false },
		"oneshot-busy": { deliverAs: "aside", triggerTurn: false },
		"oneshot-idle": { deliverAs: "nextTurn", triggerTurn: false },
	},
	deferred: {
		"interactive-busy": { deliverAs: "nextTurn", triggerTurn: false },
		"interactive-idle": { deliverAs: "nextTurn", triggerTurn: false },
		"oneshot-busy": { deliverAs: "nextTurn", triggerTurn: false },
		"oneshot-idle": { deliverAs: "nextTurn", triggerTurn: false },
	},
};

/** The phase of a session: tui and rpc are interactive; print, json and an unknown mode are one-shot. */
export function phaseOf(mode: string | undefined, idle: boolean): SessionPhase {
	if (mode === "tui" || mode === "rpc") return idle ? "interactive-idle" : "interactive-busy";
	return idle ? "oneshot-idle" : "oneshot-busy";
}

/** The mechanism a class gets in a phase. */
export function mechanismFor(cls: DeliveryClass, phase: SessionPhase): Mechanism {
	return MECHANISMS[cls][phase];
}

/** Whether the dispatcher may hold an entry of this class for coalescing. */
export function holdable(cls: DeliveryClass): boolean {
	return cls === "queue";
}
