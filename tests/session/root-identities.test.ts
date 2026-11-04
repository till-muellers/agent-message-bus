/**
 * The root-identity registry (client.md §2.3, S15): the main session publishes the credentials it acts with
 * under its root key; its subagents read them from there, never from the store or the env.
 */
import { describe, expect, test } from "bun:test";
import { createRootIdentities, createRootPublication } from "../../extension/lib/session/root-identities.ts";

const LEAD = { url: "http://bus", token: "lead-tok" };
const WORKER = { url: "http://bus", token: "worker-tok" };

describe("root identities", () => {
	test("a published identity is found under its root key only", () => {
		const roots = createRootIdentities();
		roots.publish("root", LEAD);
		expect(roots.lookup("root")).toEqual(LEAD);
		expect(roots.lookup("other")).toBeUndefined();
	});

	test("a later publish replaces the identity; withdraw removes it", () => {
		const roots = createRootIdentities();
		roots.publish("root", WORKER);
		roots.publish("root", LEAD);
		expect(roots.lookup("root")).toEqual(LEAD);
		roots.withdraw("root");
		expect(roots.lookup("root")).toBeUndefined();
	});

	test("two registries share nothing", () => {
		const first = createRootIdentities();
		first.publish("root", LEAD);
		expect(createRootIdentities().lookup("root")).toBeUndefined();
	});
});

describe("root publication of a main session", () => {
	test("publishes under the current key and withdraws the key it published before", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		publication.set("old", LEAD);
		publication.set("new", WORKER);
		expect(roots.lookup("old")).toBeUndefined();
		expect(roots.lookup("new")).toEqual(WORKER);
	});

	test("no credentials withdraw the key; no key publishes nothing", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		publication.set("root", LEAD);
		publication.set("root", undefined);
		expect(roots.lookup("root")).toBeUndefined();
		publication.set(undefined, LEAD);
		expect(roots.lookup("root")).toBeUndefined();
	});

	test("clear withdraws what was published", () => {
		const roots = createRootIdentities();
		const publication = createRootPublication(roots);
		publication.set("root", LEAD);
		publication.clear();
		expect(roots.lookup("root")).toBeUndefined();
	});
});
