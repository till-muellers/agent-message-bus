/**
 * The text a send result reads as (`omp-tools.feature` "Send results read like omp's delivery receipts", contracts.md
 * §3.4), worded like omp's own delivery receipts: a direct message says whether it reached a connected agent or
 * waits in the inbox; a topic message counts its subscribers, the live ones first. Unknown fields are ignored; a
 * result without valid receipts (an older server, a changed shape) has no receipt text, so it stays JSON.
 */
import { isSendTool } from "./outgoing-card.ts";

interface Receipt {
	to: string;
	delivery: "live" | "queued";
}

function isReceipt(value: unknown): value is Receipt {
	const receipt = value as Partial<Receipt> | null;
	return typeof receipt?.to === "string" && (receipt.delivery === "live" || receipt.delivery === "queued");
}

function receiptsOf(result: unknown): Receipt[] | undefined {
	const receipts = (result as { receipts?: unknown } | null)?.receipts;
	return Array.isArray(receipts) && receipts.every(isReceipt) ? receipts : undefined;
}

function directText(receipts: Receipt[]): string | undefined {
	if (receipts.length === 0) return undefined;
	return receipts.map(r => (r.delivery === "live" ? `Delivered to ${r.to}.` : `Queued for ${r.to}; it is not connected right now.`)).join(" ");
}

function topicText(receipts: Receipt[]): string {
	const ordered = [...receipts.filter(r => r.delivery === "live"), ...receipts.filter(r => r.delivery === "queued")];
	const list = ordered.map(r => `${r.to} ${r.delivery}`).join(", ");
	return `Delivered to ${receipts.length} subscriber(s)${list ? `: ${list}` : ""}.`;
}

/** The receipt text of a send tool's result; undefined for other tools and for results without valid receipts. */
export function receiptText(tool: string, result: unknown): string | undefined {
	if (!isSendTool(tool)) return undefined;
	const receipts = receiptsOf(result);
	if (!receipts) return undefined;
	return tool === "bus_send_direct" ? directText(receipts) : topicText(receipts);
}
