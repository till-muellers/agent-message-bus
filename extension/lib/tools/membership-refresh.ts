/**
 * The tools that change the spaces a session is in (`docs/design/arch/client.md` §5): after one of them
 * succeeded, the identity of the context block is read again before the tool returns, so the next model
 * request already sees the change.
 */

const MEMBERSHIP_TOOLS: ReadonlySet<string> = new Set(["bus_create_space", "bus_join_space", "bus_leave_space", "bus_delete_space"]);

/** Whether a successful call of the tool changes the session's spaces. */
export function changesMembership(tool: string): boolean {
	return MEMBERSHIP_TOOLS.has(tool);
}
