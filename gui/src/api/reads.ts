import { decodeAdmins } from "./decode-admins.ts";
import { decodeEntries } from "./decode-inbox.ts";
import { decodeSpaceDetail, decodeSpaceList } from "./decode-state.ts";
import { decodeThread, decodeThreadInfos } from "./decode-threads.ts";
import type { Http } from "./http.ts";
import type { AdminInfo, Entry, SpaceDetail, SpaceList, Thread, ThreadInfo } from "./types.ts";

/** One inbox read: the oldest entries, at most limit (the server's maxInboxLimit) of them. */
export interface InboxRead {
	entries: Entry[];
	limit: number;
}

/** The read-only GETs of /web/api (contracts.md §10.2). */
export interface Reads {
	spaces(): Promise<SpaceList>;
	space(name: string): Promise<SpaceDetail>;
	threads(space: string, board: string): Promise<ThreadInfo[]>;
	thread(space: string, id: number): Promise<Thread>;
	/** The signed-in agent's own inbox, oldest first, as many entries as the state allows; never acks, never opens the stream. */
	inbox(): Promise<InboxRead>;
	/** Admin sessions only; others get 403. */
	admins(): Promise<AdminInfo[]>;
}

function spacePath(space: string): string {
	return `/web/api/spaces/${encodeURIComponent(space)}`;
}

async function readInbox(http: Http): Promise<InboxRead> {
	const { maxInboxLimit: limit } = await http.get("/web/api/state", decodeSpaceList);
	return { entries: await http.get(`/web/api/inbox?limit=${limit}`, decodeEntries), limit };
}

export function createReads(http: Http): Reads {
	return {
		spaces: () => http.get("/web/api/state", decodeSpaceList),
		space: (name) => http.get(`/web/api/state?space=${encodeURIComponent(name)}`, decodeSpaceDetail),
		threads: (space, board) =>
			http.get(`${spacePath(space)}/blackboards/${encodeURIComponent(board)}/threads`, decodeThreadInfos),
		thread: (space, id) => http.get(`${spacePath(space)}/threads/${id}`, decodeThread),
		inbox: () => readInbox(http),
		admins: () => http.get("/web/api/admins", decodeAdmins),
	};
}
