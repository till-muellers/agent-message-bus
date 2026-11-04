// Wire types of the /web routes (docs/design/arch/contracts.md §10).

export const AGENT_KINDS = ["admin", "leader", "member"] as const;
export type AgentKind = (typeof AGENT_KINDS)[number];

/** The signed-in agent; a member names its one space. */
export interface Identity {
	kind: AgentKind;
	name: string;
	space?: string;
}

/** POST and GET /web/session: who is signed in and the session's CSRF token. */
export interface SessionInfo {
	identity: Identity;
	csrf: string;
}
