import { decodeDeleted, decodeOk } from "./decode.ts";
import { decodeCancelled, decodeInviteCode, decodePostId } from "./decode-mutations.ts";
import type { Http } from "./http.ts";
import type { InviteCode } from "./types.ts";

/** The space mutations of /web/api (contracts.md §10.4, §10.6); each sends the CSRF token. */
export interface Mutations {
	/** An empty task sends none. */
	invite(space: string, name: string, task: string): Promise<InviteCode>;
	cancelInvite(space: string, id: string): Promise<boolean>;
	revoke(space: string, name: string): Promise<boolean>;
	post(space: string, threadId: number, body: string): Promise<number>;
	sendTopic(space: string, topic: string, body: string, critical: boolean): Promise<true>;
	deleteSpace(space: string): Promise<true>;
}

function spacePath(space: string): string {
	return `/web/api/spaces/${encodeURIComponent(space)}`;
}

export function createMutations(http: Http): Mutations {
	return {
		invite: (space, name, task) => http.send("POST", `${spacePath(space)}/invites`, task === "" ? { name } : { name, task }, decodeInviteCode),
		cancelInvite: (space, id) => http.send("DELETE", `${spacePath(space)}/invites/${encodeURIComponent(id)}`, undefined, decodeCancelled),
		revoke: (space, name) => http.send("POST", `${spacePath(space)}/revoke`, { name, confirm: `${space}/${name}` }, decodeDeleted),
		post: (space, threadId, body) => http.send("POST", `${spacePath(space)}/threads/${threadId}/posts`, { body }, decodePostId),
		sendTopic: (space, topic, body, critical) =>
			http.send("POST", `${spacePath(space)}/topics/${encodeURIComponent(topic)}/messages`, { body, critical }, decodeOk),
		deleteSpace: (space) => http.send("DELETE", spacePath(space), { confirm: space }, decodeOk),
	};
}
