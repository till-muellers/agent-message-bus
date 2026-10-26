/**
 * The one owner of the session's credentials (client.md §2.1).
 *
 * The promoted leader token, the env credentials and the accepted invite are
 * held by one module with named operations; the callers never reach into the
 * state. The precedence is unchanged: promote > env > accepted > stored.
 */
import { describe, expect, test } from "bun:test";
import {
	createSessionCredentials,
	type ResolvedCredentials,
	type SessionCredentials,
} from "../../extension/lib/session/session-credentials.ts";
import type { ActiveCredentials, StoredCredentials } from "../../extension/lib/session/stored-credentials.ts";

const ENV: ActiveCredentials = { url: "http://env", token: "env-tok" };
const PROMOTED: ActiveCredentials = { url: "http://promoted", token: "promoted-tok" };
const ACCEPTED: ActiveCredentials = { url: "http://accepted", token: "accepted-tok" };
const STORED: StoredCredentials = { url: "http://stored", token: "stored-tok" };

describe("resolve", () => {
	test("precedence: promote > env > accepted > stored agent token", () => {
		const stored = { url: STORED.url, token: STORED.token };
		const cases: { name: string; build: () => SessionCredentials; stored?: StoredCredentials; want?: ResolvedCredentials }[] = [
			{ name: "promote wins over everything", build: () => promoteThenEnv(), stored, want: { ...PROMOTED, source: "promote" } },
			{ name: "env wins over accepted and stored", build: () => envThenAccepted(), stored, want: { ...ENV, source: "env" } },
			{ name: "accepted wins over stored", build: () => acceptedOnly(), stored, want: { ...ACCEPTED, source: "accepted" } },
			{ name: "the stored agent token is the last resort", build: () => empty(), stored, want: { url: "http://stored", token: "stored-tok", source: "stored-agent" } },
			{ name: "nothing resolves without a stored token", build: () => empty(), want: undefined },
			{ name: "an admin-only store has no agent session", build: () => empty(), stored: { url: "http://admin" }, want: undefined },
		];
		for (const c of cases) {
			expect({ name: c.name, resolved: c.build().resolve(c.stored) }).toEqual({ name: c.name, resolved: c.want });
		}
	});

	test("the stored url is trimmed", () => {
		expect(empty().resolve({ url: "http://stored/", token: "stored-tok" })).toEqual({
			url: "http://stored",
			token: "stored-tok",
			source: "stored-agent",
		});
	});

	test("the source names where the active credentials came from", () => {
		expect(promoteThenEnv().resolve(STORED)?.source).toBe("promote");
		expect(envThenAccepted().resolve(STORED)?.source).toBe("env");
		expect(acceptedOnly().resolve(STORED)?.source).toBe("accepted");
		expect(empty().resolve(STORED)?.source).toBe("stored-agent");
	});
});

describe("the named operations", () => {
	test("promote holds the leader token for this session only", () => {
		const creds = empty();
		creds.promote(PROMOTED);
		expect(creds.resolve(STORED)).toEqual({ ...PROMOTED, source: "promote" });
	});

	test("ended drops the promoted token but keeps the env credentials", () => {
		const creds = promoteThenEnv();
		creds.ended();
		expect(creds.resolve(STORED)).toEqual({ ...ENV, source: "env" });
	});

	test("a new login replaces the promoted token", () => {
		const creds = promoteThenEnv();
		creds.loggedIn();
		expect(creds.resolve(STORED)).toEqual({ ...ENV, source: "env" });
	});

	test("an accepted invite replaces the promoted and env credentials", () => {
		const creds = promoteThenEnv();
		creds.inviteAccepted();
		creds.setAccepted(ACCEPTED);
		expect(creds.resolve(STORED)).toEqual({ ...ACCEPTED, source: "accepted" });
	});

	test("logout drops the promoted and env credentials", () => {
		const creds = promoteThenEnv();
		creds.loggedOut();
		expect(creds.resolve({ url: "http://stored", token: "stored-tok" })).toEqual({
			url: "http://stored",
			token: "stored-tok",
			source: "stored-agent",
		});
		expect(creds.resolve(undefined)).toBeUndefined();
	});

	test("setAccepted replaces the accepted token", () => {
		const creds = acceptedOnly();
		creds.setAccepted({ url: "http://other", token: "other-tok" });
		expect(creds.resolve(STORED)).toEqual({ url: "http://other", token: "other-tok", source: "accepted" });
	});
});

/** A credential set with nothing in it. */
function empty(): SessionCredentials {
	return createSessionCredentials(undefined);
}

/** A credential set with the env credentials and a promoted leader token. */
function promoteThenEnv(): SessionCredentials {
	const creds = createSessionCredentials(ENV);
	creds.promote(PROMOTED);
	return creds;
}

/** A credential set with the env credentials and an accepted invite. */
function envThenAccepted(): SessionCredentials {
	const creds = createSessionCredentials(ENV);
	creds.setAccepted(ACCEPTED);
	return creds;
}

/** A credential set with only an accepted invite. */
function acceptedOnly(): SessionCredentials {
	const creds = createSessionCredentials(undefined);
	creds.setAccepted(ACCEPTED);
	return creds;
}
