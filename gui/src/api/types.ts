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

/** A participant's role in one space. */
export const ROLES = ["admin", "leader", "member"] as const;
export type Role = (typeof ROLES)[number];

/** One row of GET /web/api/state; invites (pending codes) only with leader rights in that space. */
export interface SpaceSummary {
	name: string;
	leaders: string[];
	admins: string[];
	members: number;
	joined: boolean;
	invites?: number;
}

export interface SpaceList {
	spaces: SpaceSummary[];
	inboxWaiting: number;
}

export interface Participant {
	name: string;
	role: Role;
	connected: boolean;
}

export interface Topic {
	name: string;
	createdBy: string;
	subscribed: boolean;
}

export interface Blackboard {
	name: string;
	owner: string;
	threads: number;
}

export interface RecentThread {
	id: number;
	blackboard: string;
	title: string;
	createdBy: string;
	posts: number;
	lastPostAt: string;
	subscribed: boolean;
}

/** A pending invite: never the code. */
export interface PendingInvite {
	id: string;
	name: string;
	invitedBy: string;
	expiresAt: string;
}

/** The bus_space_overview result, in the server's order. */
export interface Overview {
	space: string;
	participants: Participant[];
	topics: Topic[];
	blackboards: Blackboard[];
	recentThreads: RecentThread[];
}

/** GET /web/api/state?space=; invites only for leader-rights sessions. */
export interface SpaceDetail {
	overview: Overview;
	invites?: PendingInvite[];
	inboxWaiting: number;
}
