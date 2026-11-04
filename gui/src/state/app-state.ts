import type { Identity } from "../api/types.ts";

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

export interface AppState {
	session: SessionState;
	route: Route;
}
