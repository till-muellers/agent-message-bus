import { describe, expect, test } from "bun:test";
import { stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { createSessionStore, sessionKey } from "../../extension/lib/session/session-store.ts";
import { URL_, useTempDirs } from "../fakes.ts";

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

	test("sessionKey: the session file's name without .jsonl, nested or not", () => {
		expect(sessionKey(undefined)).toBeUndefined();
		expect(sessionKey(join("/s", "root.jsonl"))).toBe("root");
		expect(sessionKey(join("/s", "root", "sub1", "sub2.jsonl"))).toBe("sub2");
	});
});
