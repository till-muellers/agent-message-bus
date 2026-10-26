/** How the extension words its errors: bus errors as `<code>: <message>`, and the one "not logged in" hint. */
import { BusError } from "./client.ts";

export const NOT_LOGGED_IN = "Not logged in to the agent bus. Run /bus login <url> <token>.";

/** A server error as `<code>: <message>`, any other error as its message. */
export function errorMessage(error: unknown): string {
	if (error instanceof BusError) return `${error.code}: ${error.message}`;
	return error instanceof Error ? error.message : String(error);
}
