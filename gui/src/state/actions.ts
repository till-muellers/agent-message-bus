import type { ApiError } from "../api/api-error.ts";
import type { Identity } from "../api/types.ts";
import type { Route } from "./app-state.ts";
import type { ViewData } from "./view-data.ts";

export type Action =
	| { type: "session/restored"; identity: Identity; csrf: string }
	| { type: "session/signed-out"; error?: string }
	| { type: "navigate"; route: Route }
	| { type: "view/loading" }
	| { type: "view/loaded"; route: Route; data: ViewData; at: number }
	| { type: "view/failed"; route: Route; error: ApiError }
	| { type: "secret/shown"; label: string; secret: string }
	| { type: "notice/shown"; message: string; alert: boolean };
