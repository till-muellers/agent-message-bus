/** The result shape every `bus_*` tool returns to omp. */
import { errorMessage } from "../bus/errors.ts";

export interface ToolResult {
	content: { type: "text"; text: string }[];
}

/**
 * Runs a tool body: its value becomes the text `text` makes of it, else pretty JSON; an error is rethrown as
 * `errorMessage` words it.
 */
export async function toolResult(body: () => Promise<unknown>, text: (value: unknown) => string | undefined = () => undefined): Promise<ToolResult> {
	try {
		const value = await body();
		return { content: [{ type: "text", text: text(value) ?? JSON.stringify(value, null, 2) }] };
	} catch (error) {
		throw new Error(errorMessage(error), { cause: error });
	}
}
