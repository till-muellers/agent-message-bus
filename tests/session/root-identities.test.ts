/**
 * The root-identity registry (client.md §2.3, S15): the main session publishes the credentials it acts with
 * under its root key; its subagents read them from there, never from the store or the env.
 */
import { describe, expect, test } from "bun:test";
import { join } from "node:path";
import { createRootIdentities, rootKeyOf } from "../../extension/lib/session/root-identities.ts";

const LEAD = { url: "http://bus", token: "lead-tok" };
const WORKER = { url: "http://bus", token: "worker-tok" };
const SESSIONS = join("/sessions", "cwd");
const ROOT_FILE = join(SESSIONS, "root.jsonl");
const ROOT = rootKeyOf(ROOT_FILE) ?? "";

describe("root identities", () => {
	test("the root key of a main session is its file without .jsonl; a session without a file has none", () => {
		expect(ROOT.endsWith(join("cwd", "root"))).toBe(true);
		expect(rootKeyOf(undefined)).toBeUndefined();
	});

	test("a subagent finds the identity of the root session whose dir holds its file, nested or not", () => {
		const roots = createRootIdentities();
		roots.publish(ROOT, LEAD);
		expect(roots.lookup(join(SESSIONS, "root", "Explore.jsonl"))).toEqual(LEAD);
		expect(roots.lookup(join(SESSIONS, "root", "Explore", "Nested.jsonl"))).toEqual(LEAD);
	});

	test("a subagent of another root session, the root session's own file and a missing file find nothing", () => {
		const roots = createRootIdentities();
		roots.publish(ROOT, LEAD);
		expect(roots.lookup(join(SESSIONS, "other", "Explore.jsonl"))).toBeUndefined();
		expect(roots.lookup(ROOT_FILE)).toBeUndefined();
		expect(roots.lookup(undefined)).toBeUndefined();
	});

	test("a later publish replaces the identity; withdraw removes it", () => {
		const roots = createRootIdentities();
		const sub = join(SESSIONS, "root", "Explore.jsonl");
		roots.publish(ROOT, WORKER);
		roots.publish(ROOT, LEAD);
		expect(roots.lookup(sub)).toEqual(LEAD);
		roots.withdraw(ROOT);
		expect(roots.lookup(sub)).toBeUndefined();
	});

	test("two registries share nothing", () => {
		createRootIdentities().publish(ROOT, LEAD);
		expect(createRootIdentities().lookup(join(SESSIONS, "root", "Explore.jsonl"))).toBeUndefined();
	});
});
