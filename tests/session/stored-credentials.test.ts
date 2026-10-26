import { describe, expect, test } from "bun:test";
import { stat, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { configDir, createCredentialStore, resolveActive } from "../../extension/lib/session/stored-credentials.ts";
import { consumeEnv } from "../../extension/lib/session/env.ts";
import { URL_, useTempDirs } from "../fakes.ts";

describe("credentials", () => {
	const tempDir = useTempDirs();

	test("resolveActive: session override > env credentials > accepted invite > file", () => {
		const stored = { url: "http://file", token: "file-tok" };
		const env = { url: "http://env", token: "env-tok" };
		const override = { url: "http://session", token: "session-tok" };
		const accepted = { url: "http://accepted", token: "acc-tok" };
		expect(resolveActive({ override, env, accepted }, stored)).toEqual(override);
		expect(resolveActive({ env, accepted }, stored)).toEqual(env);
		expect(resolveActive({ accepted }, stored)).toEqual(accepted);
		expect(resolveActive({}, stored)).toEqual({ url: "http://file", token: "file-tok" });
		expect(resolveActive({}, { url: "http://file", adminToken: "a" })).toBeUndefined();
		expect(resolveActive({}, undefined)).toBeUndefined();
	});

	test("configDir: OMP_AGENT_BUS_DIR wins, else <PI_CODING_AGENT_DIR or ~/.omp/agent>/agent-message-bus", () => {
		expect(configDir({ OMP_AGENT_BUS_DIR: "/bus", PI_CODING_AGENT_DIR: "/agent" })).toBe("/bus");
		expect(configDir({ PI_CODING_AGENT_DIR: "/agent" })).toBe(join("/agent", "agent-message-bus"));
		expect(configDir({})).toBe(join(homedir(), ".omp", "agent", "agent-message-bus"));
	});

	test("store saves 0600, loads, clears; unreadable file is an error", async () => {
		const dir = await tempDir();
		const store = createCredentialStore({ OMP_AGENT_BUS_DIR: dir });
		expect(store.path).toBe(join(dir, "credentials.json"));
		expect(await store.load()).toBeUndefined();
		await store.save({ url: URL_, token: "t" });
		expect(await store.load()).toEqual({ url: URL_, token: "t" });
		if (process.platform !== "win32") expect((await stat(store.path)).mode & 0o777).toBe(0o600);
		expect(await store.clear()).toBe(true);
		expect(await store.clear()).toBe(false);
		await writeFile(store.path, "{nope");
		await expect(store.load()).rejects.toThrow(`Credential file ${store.path} is unreadable. Run /bus login to rewrite it.`);
	});
});

describe("consumeEnv", () => {
	test("copies url, token and bin into memory and deletes them; credentials need both url and token", () => {
		for (const [input, expected] of [
			[{ AGENT_BUS_URL: "http://a/", AGENT_BUS_TOKEN: "t", AGENT_BUS_BIN: "/b", PATH: "/p" }, { credentials: { url: "http://a", token: "t" }, bin: "/b" }],
			[{ AGENT_BUS_URL: "http://a", PATH: "/p" }, {}],
			[{ PATH: "/p" }, {}],
		] as const) {
			const env: NodeJS.ProcessEnv = { ...input };
			expect(consumeEnv(env)).toEqual(expected);
			expect(env).toEqual({ PATH: "/p" });
		}
	});
});
