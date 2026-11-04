import { decodeDeleted } from "./decode.ts";
import { decodeMinted } from "./decode-admins.ts";
import type { Http } from "./http.ts";
import type { MintedAdmin } from "./types.ts";

/** The admin mutations of /web/api/admins (contracts.md §10.3); each sends the CSRF token. */
export interface AdminsApi {
	mint(name: string): Promise<MintedAdmin>;
	/** Whether anything was removed. */
	revoke(name: string): Promise<boolean>;
}

export function createAdminsApi(http: Http): AdminsApi {
	return {
		mint: (name) => http.send("POST", "/web/api/admins", { name }, decodeMinted),
		revoke: (name) => http.send("POST", `/web/api/admins/${encodeURIComponent(name)}/revoke`, {}, decodeDeleted),
	};
}
