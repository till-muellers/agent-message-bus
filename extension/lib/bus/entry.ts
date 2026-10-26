/** The inbox entry as the server sends it: the wire type every layer reads. */
export type EntryType = "direct_message" | "topic_message" | "topic_closed" | "thread_post" | "thread_removed" | "blackboard_removed";

export interface Entry {
	id: number;
	space: string;
	type: EntryType;
	from: string;
	critical: boolean;
	topic?: string;
	blackboard?: string;
	threadId?: number;
	threadTitle?: string;
	body?: string;
	createdAt: string;
}
