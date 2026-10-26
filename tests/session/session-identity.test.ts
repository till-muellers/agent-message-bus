import { describe, expect, test } from "bun:test";
import { mkdir, readFile, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { SessionCredentials } from "../../extension/lib/session/stored-credentials.ts";
import { createSessionIdentity } from "../../extension/lib/session/session-identity.ts";
import { URL_, useTempDirs } from "../fakes.ts";

const ctx = (sessionFile: string | undefined, kind = "main") => ({ agent: { kind }, sessionManager: { getSessionFile: () => sessionFile } });
const creds = { url: URL_, token: "acc-tok" };
const NEW = { kind: "new" } as const;
const branch = (previousSessionFile: string | undefined) => ({ kind: "branch", previousSessionFile }) as const;

describe("session identity", () => {
	const tempDir = useTempDirs();

	test("save writes {url, token} under the bound session's key and makes it the accepted credentials", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(join(dir, "s1.jsonl")));
		expect(await identity.save(creds)).toBe(true);
		expect(credentials.accepted).toEqual(creds);
		expect(JSON.parse(await readFile(join(dir, "sessions", "s1.json"), "utf8"))).toEqual(creds);
	});

	test("load restores the bound session's token; a new session drops it until that one loads", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const writer = createSessionIdentity(dir, {});
		await writer.bind(ctx(join(dir, "s1.jsonl")));
		await writer.save(creds);
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(join(dir, "s1.jsonl")));
		await identity.load();
		expect(credentials.accepted).toEqual(creds);
		await identity.bind(ctx(join(dir, "s2.jsonl")), NEW);
		expect(credentials.accepted).toBeUndefined();
		await identity.load();
		expect(credentials.accepted).toBeUndefined();
	});

	test("re-binding the same session keeps the token held in memory", async () => {
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(await tempDir(), credentials);
		await identity.bind(ctx(undefined));
		await identity.save(creds);
		await identity.bind(ctx(undefined));
		expect(credentials.accepted).toEqual(creds);
	});

	test("a new session drops the token even when neither session has a file (--no-session /new)", async () => {
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(await tempDir(), credentials);
		await identity.bind(ctx(undefined));
		await identity.save(creds);
		await identity.bind(ctx(undefined), NEW);
		expect(credentials.accepted).toBeUndefined();
	});

	test("a branch carries the previous session's token to the new session file", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(join(dir, "s1.jsonl")));
		await identity.save(creds);
		await identity.bind(ctx(join(dir, "s2.jsonl")), branch(join(dir, "s1.jsonl")));
		expect(credentials.accepted).toEqual(creds);
		expect(JSON.parse(await readFile(join(dir, "sessions", "s2.json"), "utf8"))).toEqual(creds);
		expect(JSON.parse(await readFile(join(dir, "sessions", "s1.json"), "utf8"))).toEqual(creds);
	});

	test("a branch from a session that accepted nothing carries nothing", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(join(dir, "s1.jsonl")));
		await identity.bind(ctx(join(dir, "s2.jsonl")), branch(join(dir, "s1.jsonl")));
		expect(credentials.accepted).toBeUndefined();
		await expect(stat(join(dir, "sessions", "s2.json"))).rejects.toThrow();
	});

	test("a branch without session files keeps the token held in memory", async () => {
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(await tempDir(), credentials);
		await identity.bind(ctx(undefined));
		await identity.save(creds);
		await identity.bind(ctx(undefined), branch(undefined));
		expect(credentials.accepted).toEqual(creds);
	});

	test("without a session file, save keeps the token in memory only; load keeps it; a new session drops it", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(undefined));
		expect(await identity.save(creds)).toBe(false);
		await identity.load();
		expect(credentials.accepted).toEqual(creds);
		await expect(stat(join(dir, "sessions"))).rejects.toThrow();
		await identity.bind(ctx(join(dir, "s1.jsonl")), NEW);
		expect(credentials.accepted).toBeUndefined();
	});

	test("clear forgets the token and deletes the session's file", async () => {
		const dir = await tempDir();
		const credentials: SessionCredentials = {};
		const identity = createSessionIdentity(dir, credentials);
		await identity.bind(ctx(join(dir, "s1.jsonl")));
		await identity.save(creds);
		await identity.clear();
		expect(credentials.accepted).toBeUndefined();
		await expect(stat(join(dir, "sessions", "s1.json"))).rejects.toThrow();
	});

	test("a subagent binds to its root session's key", async () => {
		const dir = await tempDir();
		await writeFile(join(dir, "root.jsonl"), "");
		await mkdir(join(dir, "root"));
		const root = createSessionIdentity(dir, {});
		await root.bind(ctx(join(dir, "root.jsonl")));
		await root.save(creds);
		const credentials: SessionCredentials = {};
		const sub = createSessionIdentity(dir, credentials);
		await sub.bind(ctx(join(dir, "root", "task-1.jsonl"), "sub"));
		await sub.load();
		expect(credentials.accepted).toEqual(creds);
	});
});
