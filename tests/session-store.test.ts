import { describe, expect, test } from "bun:test";
import { mkdir, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { createSessionStore, sessionKey } from "../extension/lib/session/session-store.ts";
import { URL_, useTempDirs } from "./fakes.ts";

describe("session store", () => {
	const tempDir = useTempDirs();

	test("keeps {url, token} in <dir>/sessions/<key>.json; an incomplete file is unreadable", async () => {
		const dir = await tempDir();
		const store = createSessionStore(dir, "abc");
		expect(store.path).toBe(join(dir, "sessions", "abc.json"));
		expect(await store.load()).toBeUndefined();
		await store.save({ url: URL_, token: "acc-tok" });
		expect(await store.load()).toEqual({ url: URL_, token: "acc-tok" });
		if (process.platform !== "win32") expect((await stat(store.path)).mode & 0o777).toBe(0o600);
		await writeFile(store.path, JSON.stringify({ url: URL_ }));
		await expect(store.load()).rejects.toThrow(`Session credential file ${store.path} is unreadable. Run /bus accept again.`);
	});

	test("sessionKey: the session file's name; a subagent walks up to its interactive root session", async () => {
		const dir = await tempDir();
		await writeFile(join(dir, "root.jsonl"), "");
		await mkdir(join(dir, "root", "sub1"), { recursive: true });
		await writeFile(join(dir, "root", "sub1.jsonl"), "");
		const nested = join(dir, "root", "sub1", "sub2.jsonl");
		expect(sessionKey(undefined, false)).toBeUndefined();
		expect(sessionKey(undefined, true)).toBeUndefined();
		expect(sessionKey(join(dir, "root.jsonl"), false)).toBe("root");
		expect(sessionKey(nested, false)).toBe("sub2");
		expect(sessionKey(nested, true)).toBe("root");
		expect(sessionKey(join(dir, "lone.jsonl"), true)).toBe("lone");
	});
});
