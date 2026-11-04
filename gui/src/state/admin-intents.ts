import type { AdminsApi } from "../api/admins.ts";
import { type MutationDeps, mutate } from "./mutate.ts";

export interface AdminIntents {
	mint(name: string): Promise<void>;
	revoke(name: string): Promise<void>;
}

/** Mints (token shown once) and revokes admins; never rejects. */
export function createAdminIntents(api: AdminsApi, deps: MutationDeps): AdminIntents {
	return {
		mint: (name) =>
			mutate(deps, api.mint(name), (minted) => ({ type: "secret/shown", label: `Token of admin ${minted.name}`, secret: minted.token })),
		revoke: (name) =>
			mutate(deps, api.revoke(name), (deleted) => ({
				type: "notice/shown",
				message: deleted ? `Admin ${name} revoked.` : `Nothing was removed: admin ${name} has no token and joined no space.`,
				alert: false,
			})),
	};
}
