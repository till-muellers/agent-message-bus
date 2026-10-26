/** The result shape every `bus_*` tool returns to omp. */
import { errorMessage } from "./bus/errors.ts";

export interface ToolResult {
	content: { type: "text"; text: string }[];
}

/** Runs a tool body: its value becomes pretty JSON text; an error is rethrown as `errorMessage` words it. */
export async function toolResult(body: () => Promise<unknown>): Promise<ToolResult> {
	try {
		return { content: [{ type: "text", text: JSON.stringify(await body(), null, 2) }] };
	} catch (error) {
		throw new Error(errorMessage(error), { cause: error });
	}
}
