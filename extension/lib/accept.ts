/**
 * `/bus accept <url> <code>` (and `--bus-accept`): redeems an invite code for a member token and makes it this
 * session's identity. Restarting the connection and sending the first prompt are the caller's.
 */
import type { CommandResult } from "./shared/command.ts";
import type { BusClient } from "./bus/client.ts";
import { type SessionCredentials, trimUrl } from "./session/stored-credentials.ts";
import type { SessionIdentity } from "./session/session-identity.ts";

export interface AcceptDeps {
	client: BusClient;
	session: SessionCredentials;
	identity: Pick<SessionIdentity, "save">;
}

/** A redeemed invite: the reply to show and the first prompt the inviting leader gave the member, if any. */
export interface Joined {
	result: CommandResult;
	task?: string;
}

/** The accepted token replaces the promoted and env credentials, which would otherwise win over it. */
export async function acceptInvite(deps: AcceptDeps, rawUrl: string, code: string): Promise<Joined> {
	const url = trimUrl(rawUrl);
	const invite = await deps.client.acceptInvite(url, code);
	const persisted = await deps.identity.save({ url, token: invite.token });
	deps.session.override = undefined;
	deps.session.env = undefined;
	const note = persisted ? "" : " Not persisted: this session has no session file.";
	const result: CommandResult = { message: `Joined space ${invite.space} as member ${invite.name}.${note}`, level: "info" };
	return invite.task === undefined ? { result } : { result, task: invite.task };
}
