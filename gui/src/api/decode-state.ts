import { count, type Decoder, type Fields, fields, flag, list, optionalCount, text, textItem } from "./decode.ts";
import type { Blackboard, Overview, Participant, PendingInvite, RecentThread, SpaceDetail, SpaceList, SpaceSummary, Topic } from "./types.ts";

// Decoders of GET /web/api/state (contracts.md §10.2). "you" is not decoded: the session names the operator.

function summary(json: unknown, what: string): SpaceSummary {
	const o = fields(json, what);
	const invites = optionalCount(o, "invites", what);
	const row: SpaceSummary = {
		name: text(o, "name", what),
		leaders: list(o, "leaders", what, textItem),
		admins: list(o, "admins", what, textItem),
		members: count(o, "members", what),
		joined: flag(o, "joined", what),
		mayManage: flag(o, "mayManage", what),
	};
	return invites === undefined ? row : { ...row, invites };
}

function participant(json: unknown, what: string): Participant {
	const o = fields(json, what);
	return { name: text(o, "name", what), role: text(o, "role", what), connected: flag(o, "connected", what) };
}

function topic(json: unknown, what: string): Topic {
	const o = fields(json, what);
	return { name: text(o, "name", what), createdBy: text(o, "createdBy", what), subscribed: flag(o, "subscribed", what) };
}

function blackboard(json: unknown, what: string): Blackboard {
	const o = fields(json, what);
	return { name: text(o, "name", what), owner: text(o, "owner", what), threads: count(o, "threads", what) };
}

function recentThread(json: unknown, what: string): RecentThread {
	const o = fields(json, what);
	return {
		id: count(o, "id", what),
		blackboard: text(o, "blackboard", what),
		title: text(o, "title", what),
		createdBy: text(o, "createdBy", what),
		posts: count(o, "posts", what),
		lastPostAt: text(o, "lastPostAt", what),
		subscribed: flag(o, "subscribed", what),
	};
}

function pendingInvite(json: unknown, what: string): PendingInvite {
	const o = fields(json, what);
	return {
		id: text(o, "id", what),
		name: text(o, "name", what),
		invitedBy: text(o, "invitedBy", what),
		expiresAt: text(o, "expiresAt", what),
	};
}

function overview(json: unknown, what: string): Overview {
	const o = fields(json, what);
	return {
		space: text(o, "space", what),
		participants: list(o, "participants", what, participant),
		topics: list(o, "topics", what, topic),
		blackboards: list(o, "blackboards", what, blackboard),
		recentThreads: list(o, "recentThreads", what, recentThread),
	};
}

function inboxWaiting(body: Fields): number {
	return count(fields(body.inbox, "state.inbox"), "waiting", "state.inbox");
}

export const decodeSpaceList: Decoder<SpaceList> = (json) => {
	const body = fields(json, "state");
	return {
		spaces: list(body, "spaces", "state", summary),
		inboxWaiting: inboxWaiting(body),
		maxInboxLimit: count(body, "maxInboxLimit", "state"),
	};
};

export const decodeSpaceDetail: Decoder<SpaceDetail> = (json) => {
	const body = fields(json, "state");
	const detail: SpaceDetail = {
		overview: overview(body.overview, "state.overview"),
		mayManage: flag(body, "mayManage", "state"),
		inboxWaiting: inboxWaiting(body),
	};
	return body.invites === undefined ? detail : { ...detail, invites: list(body, "invites", "state", pendingInvite) };
};
