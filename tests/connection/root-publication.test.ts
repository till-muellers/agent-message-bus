/** A main session's entry in the root-identity registry (client.md §2.3, S15): one key at a time. */
import { describe, expect, test } from "bun:test";
import { join } from "node:path";
import { createRootPublication } from "../../extension/lib/connection/root-publication.ts";
import { createRootIdentities, rootKeyOf } from "../../extension/lib/session/root-identities.ts";

const LEAD = { url: "http://bus", token: "lead-tok" };
const WORKER = { url: "http://bus", token: "worker-tok" };
const SESSIONS = join("/sessions", "cwd");
const ROOT = rootKeyOf(join(SESSIONS, "root.jsonl")) ?? "";

describe("root publication of a main session", () => {
	const subOf = (key: string) => join(key, "Explore.jsonl");

	test("publishes under the current key and withdraws the key it published before", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		const [old, next] = [rootKeyOf(join(SESSIONS, "old.jsonl")) ?? "", rootKeyOf(join(SESSIONS, "new.jsonl")) ?? ""];
		publication.set(old, LEAD);
		publication.set(next, WORKER);
		expect(roots.lookup(subOf(old))).toBeUndefined();
		expect(roots.lookup(subOf(next))).toEqual(WORKER);
	});

	test("no credentials withdraw the key; no key publishes nothing", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		publication.set(ROOT, LEAD);
		publication.set(ROOT, undefined);
		expect(roots.lookup(subOf(ROOT))).toBeUndefined();
		publication.set(undefined, LEAD);
		expect(roots.lookup(subOf(ROOT))).toBeUndefined();
	});

	test("clear withdraws what was published", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		publication.set(ROOT, LEAD);
		publication.clear();
		expect(roots.lookup(subOf(ROOT))).toBeUndefined();
	});
});
