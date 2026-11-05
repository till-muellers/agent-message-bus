/** The credentials a request to the bus carries: the server url and the agent token it acts with. */

/** The url and agent token the session acts with. */
export interface ActiveCredentials {
	url: string;
	token: string;
}

/** The server url without trailing slashes, so paths append cleanly and two spellings compare equal. */
export function trimUrl(url: string): string {
	return url.replace(/\/+$/, "");
}
