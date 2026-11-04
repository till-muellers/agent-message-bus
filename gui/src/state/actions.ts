import type { Identity } from "../api/types.ts";
import type { Route } from "./app-state.ts";

export type Action =
	| { type: "session/restored"; identity: Identity; csrf: string }
	| { type: "session/signed-out"; error?: string }
	| { type: "navigate"; route: Route };
