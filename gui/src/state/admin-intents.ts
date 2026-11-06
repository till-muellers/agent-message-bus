import type { AdminsApi } from "../api/admins.ts";
import { type MutationDeps, mutate } from "./mutate.ts";

/** Resolves true when it went through and false when it was refused (the form keeps what was typed). */
export interface AdminIntents {
	mintAdmin(name: string): Promise<boolean>;
	revokeAdmin(name: string): Promise<boolean>;
}

/** Mints (token shown once) and revokes admins; never rejects. */
export function createAdminIntents(api: AdminsApi, deps: MutationDeps): AdminIntents {
	return {
		mintAdmin: (name) =>
			mutate(deps, api.mint(name), (minted) => ({ type: "secret/shown", label: `Token of admin ${minted.name}`, secret: minted.token })),
		revokeAdmin: (name) =>
			mutate(deps, api.revoke(name), (deleted) => ({
				type: "notice/shown",
				message: deleted ? `Admin ${name} revoked.` : `Nothing was removed: admin ${name} has no token and joined no space.`,
				alert: false,
			})),
	};
}
