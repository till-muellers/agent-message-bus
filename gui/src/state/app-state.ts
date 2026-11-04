import type { ApiError } from "../api/api-error.ts";
import type { Identity, Role } from "../api/types.ts";
import type { ViewData } from "./view-data.ts";

export type Route =
	| { name: "spaces" }
	| { name: "space"; space: string }
	| { name: "board"; space: string; board: string }
	| { name: "thread"; space: string; threadId: number }
	| { name: "inbox" }
	| { name: "admins" }
	| { name: "not-found" };

export type SessionState =
	| { status: "unknown" }
	| { status: "signed-out"; error?: string }
	| { status: "signed-in"; identity: Identity; csrf: string };

/** A loaded resource; loading and error keep the last data to show meanwhile. */
export type Resource<T> =
	| { status: "idle" }
	| { status: "loading"; last?: T }
	| { status: "ready"; data: T; at: number }
	| { status: "error"; error: ApiError; last?: T };

export interface AppState {
	session: SessionState;
	route: Route;
	/** The current route's data; dropped on navigation and sign-out. */
	view: Resource<ViewData>;
	/** An invite code or admin token, shown once; dropped on navigation and sign-out. */
	shownOnce?: ShownOnce;
	/** The outcome of the last mutation (the server's message when refused); dropped on navigation. */
	notice?: Notice;
	/** A revoke or space deletion waiting for the operator's confirmation; dropped on navigation. */
	confirm?: PendingConfirm;
}

export type PendingConfirm = { kind: "revoke"; space: string; name: string; role: Role } | { kind: "delete-space"; space: string };

export interface ShownOnce {
	label: string;
	secret: string;
}

export interface Notice {
	message: string;
	alert: boolean;
}
