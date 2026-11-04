import { isFields } from "../api/decode.ts";

/** Deep equality of JSON-like values (decoded wire data): lists by order, objects by keys. */
export function sameData(a: unknown, b: unknown): boolean {
	if (a === b) return true;
	if (Array.isArray(a) || Array.isArray(b)) {
		return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((item, i) => sameData(item, b[i]));
	}
	if (!isFields(a) || !isFields(b)) return false;
	const keys = Object.keys(a);
	return keys.length === Object.keys(b).length && keys.every((key) => key in b && sameData(a[key], b[key]));
}
