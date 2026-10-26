/** Minimal server-sent-events parser (id, event, multi-line data; comments and CRLF handled). */
export interface SseFrame {
	id?: string;
	event: string;
	data: string;
}

/** Fields collected for the frame being read; a blank line dispatches it. */
interface PendingFrame {
	id: string | undefined;
	event: string;
	data: string[];
}

/** Decodes the body and yields its lines without the line terminator; a trailing unterminated line is yielded too. */
async function* readLines(body: ReadableStream<Uint8Array>): AsyncGenerator<string> {
	const decoder = new TextDecoder();
	let buffer = "";
	for await (const chunk of body) {
		buffer += decoder.decode(chunk, { stream: true });
		let newline = buffer.indexOf("\n");
		while (newline !== -1) {
			yield buffer.slice(0, newline).replace(/\r$/, "");
			buffer = buffer.slice(newline + 1);
			newline = buffer.indexOf("\n");
		}
	}
	buffer += decoder.decode();
	if (buffer) yield buffer.replace(/\r$/, "");
}

/** Builds the frame (only if it has data) and resets `pending` for the next one. */
function dispatch(pending: PendingFrame): SseFrame | undefined {
	let frame: SseFrame | undefined;
	if (pending.data.length > 0) {
		frame = { event: pending.event || "message", data: pending.data.join("\n") };
		if (pending.id !== undefined) frame.id = pending.id;
	}
	pending.id = undefined;
	pending.event = "";
	pending.data = [];
	return frame;
}

/** Applies one non-blank line (`field: value`, `field`, or a `:` comment) to `pending`. */
function applyLine(pending: PendingFrame, line: string): void {
	if (line.startsWith(":")) return;
	const colon = line.indexOf(":");
	const field = colon === -1 ? line : line.slice(0, colon);
	const value = colon === -1 ? "" : line.slice(colon + 1).replace(/^ /, "");
	if (field === "data") pending.data.push(value);
	else if (field === "event") pending.event = value;
	else if (field === "id") pending.id = value;
}

export async function* parseSse(body: ReadableStream<Uint8Array>): AsyncGenerator<SseFrame> {
	const pending: PendingFrame = { id: undefined, event: "", data: [] };
	for await (const line of readLines(body)) {
		if (line !== "") {
			applyLine(pending, line);
			continue;
		}
		const frame = dispatch(pending);
		if (frame) yield frame;
	}
	const last = dispatch(pending);
	if (last) yield last;
}
