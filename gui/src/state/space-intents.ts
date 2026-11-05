import type { Mutations } from "../api/mutations.ts";
import type { PendingInvite } from "../api/types.ts";
import type { Action } from "./actions.ts";
import type { PendingConfirm, Route } from "./app-state.ts";
import { type MutationDeps, mutate } from "./mutate.ts";

export interface SpaceIntents {
	invite(space: string, name: string, task: string): Promise<void>;
	cancelInvite(space: string, invite: PendingInvite): Promise<void>;
	post(space: string, threadId: number, body: string): Promise<void>;
	send(space: string, topic: string, body: string, critical: boolean): Promise<void>;
	/** Opens the confirmation of a revoke or a space deletion. */
	ask(confirm: PendingConfirm): void;
	/** Runs a confirmed revoke or space deletion. */
	confirmed(confirm: PendingConfirm): Promise<void>;
	dismiss(): void;
}

function told(message: string): Action {
	return { type: "notice/shown", message, alert: false };
}

function capitalized(word: string): string {
	return word.charAt(0).toUpperCase() + word.slice(1);
}

/** Where the operator is, and how to send them elsewhere. */
export interface Navigation {
	current(): Route;
	go(route: Route): void;
}

/**
 * Runs the mutations of a space (invite code shown once, outcome as a notice); never rejects.
 * A deleted space sends an operator still on one of its pages to the space list first, so the notice stays there and
 * the dead route is no longer polled; an operator who moved on while the delete ran stays where they are.
 */
export function createSpaceIntents(api: Mutations, deps: MutationDeps, nav: Navigation): SpaceIntents {
	const spaceDeleted = (space: string): Action => {
		const here = nav.current();
		if ("space" in here && here.space === space) nav.go({ name: "spaces" });
		return told(`Space ${space} deleted.`);
	};
	const run = (confirm: PendingConfirm): Promise<void> => {
		if (confirm.kind === "delete-space") return mutate(deps, api.deleteSpace(confirm.space), () => spaceDeleted(confirm.space));
		const { space, name, role } = confirm;
		const removed = `${capitalized(role)} ${name} revoked from space ${space}.`;
		return mutate(deps, api.revoke(space, name), (deleted) => told(deleted ? removed : `Nothing was removed: ${name} is not in space ${space}.`));
	};
	return {
		invite: (space, name, task) =>
			mutate(deps, api.invite(space, name, task), (r) => ({ type: "secret/shown", label: `Invite code for ${r.name} in space ${r.space}`, secret: r.code })),
		cancelInvite: (space, invite) =>
			mutate(deps, api.cancelInvite(space, invite.id), (cancelled) =>
				told(cancelled ? `Invite for ${invite.name} cancelled.` : "The invite was already used, expired or cancelled."),
			),
		post: (space, threadId, body) => mutate(deps, api.post(space, threadId, body), (id) => told(`Post ${id} added.`)),
		send: (space, topic, body, critical) => mutate(deps, api.sendTopic(space, topic, body, critical), () => told(`Sent to topic ${topic}.`)),
		ask: (confirm) => deps.dispatch({ type: "confirm/open", confirm }),
		confirmed: (confirm) => {
			deps.dispatch({ type: "confirm/close" });
			return run(confirm);
		},
		dismiss: () => deps.dispatch({ type: "confirm/close" }),
	};
}
