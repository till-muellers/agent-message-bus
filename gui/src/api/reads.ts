import { decodeSpaceDetail, decodeSpaceList } from "./decode-state.ts";
import type { Http } from "./http.ts";
import type { SpaceDetail, SpaceList } from "./types.ts";

/** The read-only GETs of /web/api (contracts.md §10.2). */
export interface Reads {
	spaces(): Promise<SpaceList>;
	space(name: string): Promise<SpaceDetail>;
}

export function createReads(http: Http): Reads {
	return {
		spaces: () => http.get("/web/api/state", decodeSpaceList),
		space: (name) => http.get(`/web/api/state?space=${encodeURIComponent(name)}`, decodeSpaceDetail),
	};
}
