import { describe, expect, test } from "bun:test";
import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PLUGIN_NAME, isOlderVersion, pluginVersion } from "../../extension/lib/version/plugin-version.ts";

describe("pluginVersion", () => {
	test("reads the version of the package.json it is given", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-plugin-version-"));
		const file = join(dir, "package.json");
		await writeFile(file, JSON.stringify({ name: "other", version: "0.2.0" }));
		expect(await pluginVersion(file)).toBe("0.2.0");
	});

	test("the shipped package.json holds the version this plugin implements", async () => {
		expect(await pluginVersion()).toMatch(/^\d+\.\d+\.\d+/);
	});

	test("a package.json without a version is an error", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-plugin-version-"));
		const file = join(dir, "package.json");
		await writeFile(file, JSON.stringify({ name: "other" }));
		await expect(pluginVersion(file)).rejects.toThrow(`no version in ${file}`);
	});
});

describe("the plugin name", () => {
	test("is the name of the shipped package.json", async () => {
		const dir = await mkdtemp(join(tmpdir(), "amb-plugin-version-"));
		const file = join(dir, "package.json");
		await writeFile(file, JSON.stringify({ name: PLUGIN_NAME, version: "0.2.0" }));
		expect(await pluginVersion(file)).toBe("0.2.0");
		expect(PLUGIN_NAME).toBe("@till-muellers/omp-agent-message-bus");
	});
});

describe("isOlderVersion", () => {
	test("compares the numeric parts of the versions", () => {
		expect(isOlderVersion("0.1.0", "0.2.0")).toBe(true);
		expect(isOlderVersion("0.1.9", "0.2.0")).toBe(true);
		expect(isOlderVersion("0.2.0", "0.2.0")).toBe(false);
		expect(isOlderVersion("0.3.0", "0.2.0")).toBe(false);
		expect(isOlderVersion("1.0.0", "0.9.9")).toBe(false);
	});

	test("a version with fewer parts is older when the parts it has are equal", () => {
		expect(isOlderVersion("0.1", "0.1.1")).toBe(true);
		expect(isOlderVersion("0.1.1", "0.1")).toBe(false);
	});
});
