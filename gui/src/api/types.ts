// Wire types of the /web routes (docs/design/arch/contracts.md §10).

/** The signed-in agent; a member names its one space. kind stays text ("admin", "leader", "member") so a newer server's kinds still show. */
export interface Identity {
	kind: string;
	name: string;
	space?: string;
}

/** POST and GET /web/session: who is signed in and the session's CSRF token. */
export interface SessionInfo {
	identity: Identity;
	csrf: string;
}

/** One row of GET /web/api/state; mayManage is leader rights in that space, invites (pending codes) only with them. */
export interface SpaceSummary {
	name: string;
	leaders: string[];
	admins: string[];
	members: number;
	joined: boolean;
	mayManage: boolean;
	invites?: number;
}

/** GET /web/api/state; maxInboxLimit is the most entries one inbox read returns. */
export interface SpaceList {
	spaces: SpaceSummary[];
	inboxWaiting: number;
	maxInboxLimit: number;
}

/** A participant of a space; role stays text ("admin", "leader", "member") so a newer server's roles still show. */
export interface Participant {
	name: string;
	role: string;
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

/** GET /web/api/state?space=; mayManage is leader rights in the space, invites only with them. */
export interface SpaceDetail {
	overview: Overview;
	mayManage: boolean;
	invites?: PendingInvite[];
	inboxWaiting: number;
}

/** One row of bus_list_threads: a thread of a blackboard, most recently active first. */
export interface ThreadInfo {
	id: number;
	title: string;
	createdBy: string;
	posts: number;
	lastPostAt: string;
}

export interface Post {
	id: number;
	author: string;
	body: string;
	createdAt: string;
}

/** The bus_read_thread result: the thread with all its posts in order. */
export interface Thread {
	id: number;
	blackboard: string;
	title: string;
	createdBy: string;
	subscribed: boolean;
	posts: Post[];
}

/** One inbox entry (contracts.md §4); type and class stay text so a newer server's values still show. */
export interface Entry {
	id: number;
	space: string;
	type: string;
	from: string;
	critical: boolean;
	/** The delivery class (contracts.md §4.1): interrupt, queue or deferred. */
	class: string;
	topic?: string;
	blackboard?: string;
	threadId?: number;
	threadTitle?: string;
	body?: string;
	createdAt: string;
}

/** One row of bus_list_admins. */
export interface AdminInfo {
	name: string;
	bootstrap: boolean;
	tokens: number;
	spaces: string[];
}

/** The bus_mint_admin result: the token is shown once and never again. */
export interface MintedAdmin {
	name: string;
	token: string;
}

/** The bus_invite_member result: the code is shown once and never again. */
export interface InviteCode {
	name: string;
	space: string;
	code: string;
	expiresAt: string;
}
