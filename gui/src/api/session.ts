import { decodeOk, decodeSession } from "./decode.ts";
import type { Http } from "./http.ts";
import type { SessionInfo } from "./types.ts";

const SESSION = "/web/session";

export interface SessionApi {
	signIn(token: string): Promise<SessionInfo>;
	restore(): Promise<SessionInfo>;
	signOut(): Promise<void>;
}

export function createSessionApi(http: Http): SessionApi {
	return {
		signIn: (token) => http.exempt("POST", SESSION, { Authorization: `Bearer ${token}` }, decodeSession),
		restore: () => http.get(SESSION, decodeSession),
		signOut: async () => {
			await http.exempt("DELETE", SESSION, {}, decodeOk);
		},
	};
}
