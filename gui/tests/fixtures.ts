import type { Entry, SpaceDetail, SpaceList, Thread, ThreadInfo } from "../src/api/types.ts";

export const spaceList: SpaceList = {
	spaces: [
		{ name: "demo", leaders: ["lead", "lead2"], admins: ["ops"], members: 1, joined: false, invites: 1 },
		{ name: "other", leaders: ["lead"], admins: [], members: 0, joined: false, invites: 0 },
	],
	inboxWaiting: 2,
};

export const spaceDetail: SpaceDetail = {
	overview: {
		space: "demo",
		participants: [
			{ name: "ops", role: "admin", connected: false },
			{ name: "lead", role: "leader", connected: true },
			{ name: "worker", role: "member", connected: false },
		],
		topics: [
			{ name: "all", createdBy: "lead", subscribed: false },
			{ name: "notes", createdBy: "worker", subscribed: false },
		],
		blackboards: [
			{ name: "alpha", owner: "worker", threads: 0 },
			{ name: "zeta", owner: "worker", threads: 1 },
		],
		recentThreads: [
			{
				id: 7,
				blackboard: "zeta",
				title: "plan",
				createdBy: "worker",
				posts: 2,
				lastPostAt: "2026-11-04T20:00:00Z",
				subscribed: false,
			},
		],
	},
	invites: [{ id: "3f9a0c1d2e4b5a67", name: "new", invitedBy: "lead", expiresAt: "2026-11-04T20:10:00Z" }],
	inboxWaiting: 0,
};

export const threadInfos: ThreadInfo[] = [
	{ id: 2, title: "other", createdBy: "worker2", posts: 1, lastPostAt: "2026-11-04T20:05:00Z" },
	{ id: 1, title: "plan", createdBy: "worker", posts: 1, lastPostAt: "2026-11-04T20:00:00Z" },
];

export const thread: Thread = {
	id: 1,
	blackboard: "b",
	title: "plan",
	createdBy: "worker",
	subscribed: false,
	posts: [
		{ id: 1, author: "worker", body: "first", createdAt: "2026-11-04T20:00:00Z" },
		{ id: 2, author: "lead", body: "reply", createdAt: "2026-11-04T20:01:00Z" },
	],
};

export const entries: Entry[] = [
	{ id: 4, space: "demo", type: "direct_message", from: "worker", critical: false, body: "need a decision", createdAt: "2026-11-04T20:00:00Z" },
	{
		id: 9,
		space: "demo",
		type: "thread_post",
		from: "lead",
		critical: true,
		threadId: 1,
		threadTitle: "plan",
		body: "reply",
		createdAt: "2026-11-04T20:01:00Z",
	},
];
