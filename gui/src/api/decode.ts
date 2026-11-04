import { ApiError } from "./api-error.ts";
import { AGENT_KINDS, type SessionInfo } from "./types.ts";

/** Narrows a parsed JSON body to T or throws ApiError(0, "invalid_response"). */
export type Decoder<T> = (json: unknown) => T;
type Fields = Readonly<Record<string, unknown>>;

function invalid(what: string): ApiError {
	return new ApiError(0, "invalid_response", `invalid response from the server: ${what}`);
}

export function isFields(json: unknown): json is Fields {
	return typeof json === "object" && json !== null && !Array.isArray(json);
}

function fields(json: unknown, what: string): Fields {
	if (!isFields(json)) throw invalid(`${what} is not an object`);
	return json;
}

function text(obj: Fields, key: string, what: string): string {
	const value = obj[key];
	if (typeof value !== "string") throw invalid(`${what}.${key} is not text`);
	return value;
}

function optionalText(obj: Fields, key: string, what: string): string | undefined {
	return obj[key] === undefined ? undefined : text(obj, key, what);
}

function oneOf<T extends string>(obj: Fields, key: string, allowed: readonly T[], what: string): T {
	const match = allowed.find((value) => value === obj[key]);
	if (match === undefined) throw invalid(`${what}.${key} is not one of ${allowed.join(", ")}`);
	return match;
}

export const decodeSession: Decoder<SessionInfo> = (json) => {
	const body = fields(json, "session");
	const space = optionalText(body, "space", "session");
	const kind = oneOf(body, "kind", AGENT_KINDS, "session");
	const name = text(body, "name", "session");
	return { identity: space === undefined ? { kind, name } : { kind, name, space }, csrf: text(body, "csrf", "session") };
};

export const decodeOk: Decoder<true> = (json) => {
	if (fields(json, "result").ok !== true) throw invalid("result.ok is not true");
	return true;
};
