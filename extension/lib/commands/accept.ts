/**
 * `/bus accept <url> <code>` (and `--bus-accept`): redeems an invite code for a member token and makes it this
 * session's identity. Restarting the connection and sending the first prompt are the caller's.
 */
import type { CommandResult } from "../shared/command.ts";
import type { BusClient } from "../bus/client.ts";
import type { SessionCredentials } from "../session/session-credentials.ts";
import { trimUrl } from "../session/stored-credentials.ts";
import type { SessionIdentity } from "../session/session-identity.ts";
import type { ServerGate } from "../connection/server-gate.ts";

export interface AcceptDeps {
	client: BusClient;
	/** The credential set of this session; only the accept operation is called. */
	session: Pick<SessionCredentials, "inviteAccepted">;
	identity: Pick<SessionIdentity, "save">;
	/** Keeps the extension off a server that is too old; checked before the code is posted. */
	gate: ServerGate;
}

/** A redeemed invite: the reply to show. The first prompt of the inviting leader arrives as a direct message. */
export interface Joined {
	result: CommandResult;
}

/** The accepted token replaces the promoted and env credentials, which would otherwise win over it. */
export async function acceptInvite(deps: AcceptDeps, rawUrl: string, code: string): Promise<Joined> {
	const url = trimUrl(rawUrl);
	// A server that is too old gets no code: the invite stays unused.
	const verdict = await deps.gate.check(url);
	if (verdict !== "ok") throw new Error(verdict.refused);
	const invite = await deps.client.acceptInvite(url, code);
	const persisted = await deps.identity.save({ url, token: invite.token });
	deps.session.inviteAccepted();
	const note = persisted ? "" : " Not persisted: this session has no session file.";
	const result: CommandResult = { message: `Joined space ${invite.space} as member ${invite.name}.${note}`, level: "info" };
	return { result };
}
