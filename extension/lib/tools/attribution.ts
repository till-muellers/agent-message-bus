/**
 * The attribution of a subagent: the server records the token's agent as the author, so the
 * extension puts the subagent in front of the body it wrote (`docs/design/arch/client.md` §6.3);
 * `tool-policy.ts` marks the tools whose body is attributed.
 */

/** The arguments of a call with the attribution line of the subagent in front of the body. */
export function attribute(args: Record<string, unknown>, agentId: string): Record<string, unknown> {
	const body = args.body;
	if (typeof body !== "string") return args;
	return { ...args, body: `[via subagent ${agentId}]\n${body}` };
}
