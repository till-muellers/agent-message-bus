/** The inbox entry as the server sends it: the wire type every layer reads. */
export type EntryType = "direct_message" | "topic_message" | "topic_closed" | "thread_post" | "thread_removed" | "blackboard_removed";

/** How urgently an entry should reach the agent; the server decides it (`docs/design/specs/mail/delivery-class.feature`). */
export type DeliveryClass = "interrupt" | "queue" | "deferred";

/** The wire value to its class; a missing or unknown value is "queue" (never interrupts, never stalls the watermark). */
export function parseDeliveryClass(value: unknown): DeliveryClass {
	return value === "interrupt" || value === "queue" || value === "deferred" ? value : "queue";
}

export interface Entry {
	id: number;
	space: string;
	type: EntryType;
	from: string;
	critical: boolean;
	/** The delivery class of the entry; missing on a server that speaks no protocol 2. */
	class?: DeliveryClass;
	topic?: string;
	blackboard?: string;
	threadId?: number;
	threadTitle?: string;
	body?: string;
	createdAt: string;
}
