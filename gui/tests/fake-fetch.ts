import type { FetchFn } from "../src/api/http.ts";

export interface RecordedRequest {
	path: string;
	method: string;
	headers: Record<string, string>;
	body: string | undefined;
	credentials: RequestCredentials | undefined;
}

export interface FakeFetch {
	fetchFn: FetchFn;
	requests: RecordedRequest[];
}

/** A fetch that records every request and answers each with the next response. */
export function fakeFetch(...responses: (Response | Error)[]): FakeFetch {
	const requests: RecordedRequest[] = [];
	const fetchFn: FetchFn = async (path, init) => {
		requests.push({
			path,
			method: init.method ?? "GET",
			headers: Object.fromEntries(new Headers(init.headers).entries()),
			body: typeof init.body === "string" ? init.body : undefined,
			credentials: init.credentials,
		});
		const next = responses.shift();
		if (next === undefined) throw new Error(`unexpected request ${path}`);
		if (next instanceof Error) throw next;
		return next;
	};
	return { fetchFn, requests };
}

export function json(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}
