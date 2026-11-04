import { ApiError } from "./api-error.ts";
import { AGENT_KINDS, type SessionInfo } from "./types.ts";

/** Narrows a parsed JSON body to T or throws ApiError(0, "invalid_response"). */
export type Decoder<T> = (json: unknown) => T;
export type Fields = Readonly<Record<string, unknown>>;

function invalid(what: string): ApiError {
	return new ApiError(0, "invalid_response", `invalid response from the server: ${what}`);
}

export function isFields(json: unknown): json is Fields {
	return typeof json === "object" && json !== null && !Array.isArray(json);
}

export function fields(json: unknown, what: string): Fields {
	if (!isFields(json)) throw invalid(`${what} is not an object`);
	return json;
}

export function text(obj: Fields, key: string, what: string): string {
	const value = obj[key];
	if (typeof value !== "string") throw invalid(`${what}.${key} is not text`);
	return value;
}

export function optionalText(obj: Fields, key: string, what: string): string | undefined {
	return obj[key] === undefined ? undefined : text(obj, key, what);
}

export function flag(obj: Fields, key: string, what: string): boolean {
	const value = obj[key];
	if (typeof value !== "boolean") throw invalid(`${what}.${key} is not true or false`);
	return value;
}

/** A non-negative integer: counts. */
export function count(obj: Fields, key: string, what: string): number {
	const value = obj[key];
	if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) throw invalid(`${what}.${key} is not a count`);
	return value;
}

export function optionalCount(obj: Fields, key: string, what: string): number | undefined {
	return obj[key] === undefined ? undefined : count(obj, key, what);
}

export function oneOf<T extends string>(obj: Fields, key: string, allowed: readonly T[], what: string): T {
	const match = allowed.find((value) => value === obj[key]);
	if (match === undefined) throw invalid(`${what}.${key} is not one of ${allowed.join(", ")}`);
	return match;
}

/** obj[key] as a list, each element narrowed by item (which gets the element's path for messages). */
export function list<T>(obj: Fields, key: string, what: string, item: (json: unknown, what: string) => T): T[] {
	return listOf(obj[key], `${what}.${key}`, item);
}

/** A body that is itself a list, each element narrowed by item. */
export function listOf<T>(json: unknown, what: string, item: (json: unknown, what: string) => T): T[] {
	if (!Array.isArray(json)) throw invalid(`${what} is not a list`);
	return json.map((element: unknown, index) => item(element, `${what}[${index}]`));
}

export function textItem(json: unknown, what: string): string {
	if (typeof json !== "string") throw invalid(`${what} is not text`);
	return json;
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
