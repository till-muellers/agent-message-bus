import { ApiError, errorMessage } from "./api-error.ts";
import { type Decoder, isFields } from "./decode.ts";

export type FetchFn = (path: string, init: RequestInit) => Promise<Response>;
export type Method = "POST" | "DELETE";

export interface Http {
	get<T>(path: string, decode: Decoder<T>): Promise<T>;
	/** A mutation: JSON body and the session's X-CSRF-Token. */
	send<T>(method: Method, path: string, body: unknown, decode: Decoder<T>): Promise<T>;
	/** A CSRF-exempt request without a body (sign-in, sign-out). */
	exempt<T>(method: Method, path: string, headers: Readonly<Record<string, string>>, decode: Decoder<T>): Promise<T>;
}

export interface HttpDeps {
	fetchFn: FetchFn;
	csrf(): string | undefined;
}

const JSON_TYPE = "application/json";

export function createHttp(deps: HttpDeps): Http {
	const call = <T>(path: string, init: RequestInit, decode: Decoder<T>): Promise<T> =>
		request(deps.fetchFn, path, init, decode);
	return {
		get: (path, decode) => call(path, { method: "GET", headers: { Accept: JSON_TYPE } }, decode),
		send: (method, path, body, decode) =>
			call(path, { method, headers: mutationHeaders(deps.csrf()), body: JSON.stringify(body) }, decode),
		exempt: (method, path, headers, decode) => call(path, { method, headers: { Accept: JSON_TYPE, ...headers } }, decode),
	};
}

function mutationHeaders(csrf: string | undefined): Record<string, string> {
	const headers: Record<string, string> = { Accept: JSON_TYPE, "Content-Type": JSON_TYPE };
	if (csrf !== undefined) headers["X-CSRF-Token"] = csrf;
	return headers;
}

async function request<T>(fetchFn: FetchFn, path: string, init: RequestInit, decode: Decoder<T>): Promise<T> {
	let response: Response;
	try {
		response = await fetchFn(path, { ...init, credentials: "same-origin" });
	} catch (err) {
		throw new ApiError(0, "network", `the server is unreachable: ${errorMessage(err)}`);
	}
	const json = await readJson(response);
	if (!response.ok) throw failure(response.status, json);
	if (json === undefined) {
		throw new ApiError(response.status, "invalid_response", "invalid response from the server: not JSON");
	}
	return decode(json);
}

async function readJson(response: Response): Promise<unknown> {
	try {
		return JSON.parse(await response.text());
	} catch {
		return undefined;
	}
}

/** The shared envelope {"error": {"code", "message"}}, or the bare status without one. */
function failure(status: number, json: unknown): ApiError {
	const error = isFields(json) && isFields(json.error) ? json.error : {};
	const code = typeof error.code === "string" ? error.code : "http_error";
	const message = typeof error.message === "string" ? error.message : `the server answered ${status}`;
	return new ApiError(status, code, message);
}
