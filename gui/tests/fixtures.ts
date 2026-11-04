import type { SpaceDetail, SpaceList } from "../src/api/types.ts";

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
