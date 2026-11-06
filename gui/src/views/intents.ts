import type { PendingInvite } from "../api/types.ts";
import type { PendingConfirm } from "../state/app-state.ts";

/**
 * What the operator can do from a page: the one intents contract of the views, each of which takes the Pick it needs.
 * The intent modules in state/ implement it structurally (state never imports views, gui/tests/layout.test.ts);
 * main.ts only composes them.
 */
export interface Intents {
	signIn(token: string): void;
	signOut(): void;
	/** The form intents (mint, invite, post, send) resolve true when it went through: the form clears only then. */
	mintAdmin(name: string): Promise<boolean>;
	revokeAdmin(name: string): void;
	invite(space: string, name: string, task: string): Promise<boolean>;
	cancelInvite(space: string, invite: PendingInvite): void;
	post(space: string, threadId: number, body: string): Promise<boolean>;
	send(space: string, topic: string, body: string, critical: boolean): Promise<boolean>;
	/** Opens the confirmation of a revoke or a space deletion. */
	ask(confirm: PendingConfirm): void;
	/** Runs a confirmed revoke or space deletion. */
	confirmed(confirm: PendingConfirm): void;
	dismiss(): void;
}
