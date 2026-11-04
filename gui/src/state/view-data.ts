import type { AdminInfo, Entry, SpaceDetail, SpaceList, Thread, ThreadInfo } from "../api/types.ts";

/** The loaded data of the current route, tagged by the view that shows it. */
export type ViewData =
	| { kind: "spaces"; list: SpaceList }
	| { kind: "space"; detail: SpaceDetail }
	| { kind: "board"; space: string; board: string; threads: ThreadInfo[] }
	| { kind: "thread"; space: string; thread: Thread }
	| { kind: "inbox"; entries: Entry[] }
	| { kind: "admins"; admins: AdminInfo[] };
