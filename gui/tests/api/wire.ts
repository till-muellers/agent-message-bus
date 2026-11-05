import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { isFields } from "../../src/api/decode.ts";

// The /web wire fixtures, marshalled from the real Go handlers by
// server/internal/server/web_contract_test.go (regenerate with its -update flag).

const DIR = join(import.meta.dir, "..", "..", "..", "server", "internal", "web", "testdata");

/** The fixture files, sorted. */
export function wireNames(): string[] {
	return readdirSync(DIR).sort();
}

/** The parsed body of fixture name (e.g. "inbox.json"). */
export function wire(name: string): unknown {
	return JSON.parse(readFileSync(join(DIR, name), "utf8"));
}

/** The value at path inside json (list steps are indexes), undefined where a step is missing. */
export function at(json: unknown, ...path: string[]): unknown {
	return path.reduce<unknown>((value, key) => (Array.isArray(value) ? value[Number(key)] : isFields(value) ? value[key] : undefined), json);
}

/** The fields of json without key; no fields when json is no object. */
export function without(json: unknown, key: string): Record<string, unknown> {
	return isFields(json) ? Object.fromEntries(Object.entries(json).filter(([k]) => k !== key)) : {};
}
