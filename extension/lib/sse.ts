/** Minimal server-sent-events parser (id, event, multi-line data; comments and CRLF handled). */
export interface SseFrame {
	id?: string;
	event: string;
	data: string;
}

export async function* parseSse(body: ReadableStream<Uint8Array>): AsyncGenerator<SseFrame> {
	const decoder = new TextDecoder();
	let buffer = "";
	let id: string | undefined;
	let event = "";
	let data: string[] = [];
	let hasData = false;

	function* line(text: string): Generator<SseFrame> {
		if (text === "") {
			if (hasData) {
				const frame: SseFrame = { event: event || "message", data: data.join("\n") };
				if (id !== undefined) frame.id = id;
				yield frame;
			}
			id = undefined;
			event = "";
			data = [];
			hasData = false;
			return;
		}
		if (text.startsWith(":")) return;
		const colon = text.indexOf(":");
		const field = colon === -1 ? text : text.slice(0, colon);
		let value = colon === -1 ? "" : text.slice(colon + 1);
		if (value.startsWith(" ")) value = value.slice(1);
		if (field === "data") {
			data.push(value);
			hasData = true;
		} else if (field === "event") event = value;
		else if (field === "id") id = value;
	}

	for await (const chunk of body) {
		buffer += decoder.decode(chunk, { stream: true });
		let newline = buffer.indexOf("\n");
		while (newline !== -1) {
			yield* line(buffer.slice(0, newline).replace(/\r$/, ""));
			buffer = buffer.slice(newline + 1);
			newline = buffer.indexOf("\n");
		}
	}
	buffer += decoder.decode();
	if (buffer) yield* line(buffer.replace(/\r$/, ""));
	yield* line("");
}
