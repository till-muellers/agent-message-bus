/** HTTP client for the agent-message-bus web API (`/v1/tools`, `/v1/inbox/stream`). */
import type { JsonSchema } from "./schema.ts";

export interface FetchInit {
	method: string;
	headers: Record<string, string>;
	body?: string;
	signal?: AbortSignal;
}

export type FetchLike = (url: string, init: FetchInit) => Promise<Response>;

/** A server error: `code` is the server's error kind (`invalid`, `unauthenticated`, `forbidden`, …). */
export class BusError extends Error {
	constructor(
		readonly code: string,
		message: string,
	) {
		super(message);
		this.name = "BusError";
	}
}

export interface RemoteTool {
	name: string;
	description: string;
	inputSchema: JsonSchema;
}

export interface BusClient {
	listTools(url: string, token: string): Promise<RemoteTool[]>;
	callTool(url: string, token: string, name: string, args: Record<string, unknown>): Promise<unknown>;
	openStream(url: string, token: string, signal: AbortSignal): Promise<Response>;
}

async function toBusError(response: Response): Promise<BusError> {
	const text = await response.text().catch(() => "");
	try {
		const parsed = JSON.parse(text) as { error?: { code?: unknown; message?: unknown } };
		if (typeof parsed.error?.code === "string" && typeof parsed.error.message === "string")
			return new BusError(parsed.error.code, parsed.error.message);
	} catch {
		// Not a JSON error body; fall through.
	}
	return new BusError(`http_${response.status}`, text || response.statusText || `HTTP ${response.status}`);
}

export function createClient(fetchFn: FetchLike = fetch): BusClient {
	async function request(url: string, init: FetchInit): Promise<Response> {
		const response = await fetchFn(url, init);
		if (!response.ok) throw await toBusError(response);
		return response;
	}
	const auth = (token: string) => ({ Authorization: `Bearer ${token}` });
	return {
		async listTools(url, token) {
			const response = await request(`${url}/v1/tools`, { method: "GET", headers: auth(token) });
			return (await response.json()) as RemoteTool[];
		},
		async callTool(url, token, name, args) {
			const response = await request(`${url}/v1/tools/${encodeURIComponent(name)}`, {
				method: "POST",
				headers: { ...auth(token), "Content-Type": "application/json" },
				body: JSON.stringify(args),
			});
			const body: unknown = await response.json();
			if (!body || typeof body !== "object" || !("result" in body)) throw new BusError("invalid_response", `${name}: response has no result`);
			return body.result;
		},
		openStream(url, token, signal) {
			return request(`${url}/v1/inbox/stream`, { method: "GET", headers: { ...auth(token), Accept: "text/event-stream" }, signal });
		},
	};
}
