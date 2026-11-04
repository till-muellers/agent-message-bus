import type { SpaceDetail, SpaceList } from "../api/types.ts";

/** The loaded data of the current route, tagged by the view that shows it. */
export type ViewData = { kind: "spaces"; list: SpaceList } | { kind: "space"; detail: SpaceDetail };
