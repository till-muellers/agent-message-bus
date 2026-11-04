import { decodeEntries, INBOX_LIMIT } from "./decode-inbox.ts";
import { decodeSpaceDetail, decodeSpaceList } from "./decode-state.ts";
import { decodeThread, decodeThreadInfos } from "./decode-threads.ts";
import type { Http } from "./http.ts";
import type { Entry, SpaceDetail, SpaceList, Thread, ThreadInfo } from "./types.ts";

/** The read-only GETs of /web/api (contracts.md §10.2). */
export interface Reads {
	spaces(): Promise<SpaceList>;
	space(name: string): Promise<SpaceDetail>;
	threads(space: string, board: string): Promise<ThreadInfo[]>;
	thread(space: string, id: number): Promise<Thread>;
	/** The signed-in agent's own inbox, oldest first, at most INBOX_LIMIT entries; never acks, never opens the stream. */
	inbox(): Promise<Entry[]>;
}

function spacePath(space: string): string {
	return `/web/api/spaces/${encodeURIComponent(space)}`;
}

export function createReads(http: Http): Reads {
	return {
		spaces: () => http.get("/web/api/state", decodeSpaceList),
		space: (name) => http.get(`/web/api/state?space=${encodeURIComponent(name)}`, decodeSpaceDetail),
		threads: (space, board) =>
			http.get(`${spacePath(space)}/blackboards/${encodeURIComponent(board)}/threads`, decodeThreadInfos),
		thread: (space, id) => http.get(`${spacePath(space)}/threads/${id}`, decodeThread),
		inbox: () => http.get(`/web/api/inbox?limit=${INBOX_LIMIT}`, decodeEntries),
	};
}
