import { describe, expect, test } from "bun:test";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { PLUGIN_NAME, isOlderVersion, pluginVersion } from "../../extension/lib/version/plugin-version.ts";
import { useTempDirs } from "../fakes.ts";

describe("pluginVersion", () => {
	const tempDir = useTempDirs();

	test("reads the version of the package.json it is given", async () => {
		const dir = await tempDir();
		const file = join(dir, "package.json");
		await writeFile(file, JSON.stringify({ name: "other", version: "0.2.0" }));
		expect(await pluginVersion(file)).toBe("0.2.0");
	});

	test("the shipped package.json holds the version this plugin implements", async () => {
		expect(await pluginVersion()).toMatch(/^\d+\.\d+\.\d+/);
	});

	test("a package.json without a version is an error", async () => {
		const dir = await tempDir();
		const file = join(dir, "package.json");
		await writeFile(file, JSON.stringify({ name: "other" }));
		await expect(pluginVersion(file)).rejects.toThrow(`no version in ${file}`);
	});
});

describe("the plugin name", () => {
	const tempDir = useTempDirs();

	test("is the name of the shipped package.json", async () => {
		const dir = await tempDir();
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

	test("a pre-release is older than its release and newer than the release before", () => {
		expect(isOlderVersion("1.2.3-rc.1", "1.2.3")).toBe(true);
		expect(isOlderVersion("1.2.3", "1.2.3-rc.1")).toBe(false);
		expect(isOlderVersion("1.2.3-rc.1", "1.2.4")).toBe(true);
		expect(isOlderVersion("1.2.4-rc.1", "1.2.3")).toBe(false);
		expect(isOlderVersion("1.2.2", "1.2.3-rc.1")).toBe(true);
	});

	test("pre-releases follow the SemVer 2.0.0 precedence chain", () => {
		const chain = ["1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0"];
		for (let i = 0; i < chain.length; i++) {
			for (let j = 0; j < chain.length; j++) {
				expect(isOlderVersion(chain[i] ?? "", chain[j] ?? "")).toBe(i < j);
			}
		}
	});

	test("numeric identifiers compare numerically, alphanumeric ones in ASCII order, numeric before alphanumeric", () => {
		expect(isOlderVersion("1.0.0-2", "1.0.0-10")).toBe(true);
		expect(isOlderVersion("1.0.0-10", "1.0.0-2")).toBe(false);
		expect(isOlderVersion("1.0.0-1", "1.0.0-a")).toBe(true);
		expect(isOlderVersion("1.0.0-Z", "1.0.0-a")).toBe(true);
		expect(isOlderVersion("1.0.0-prerelease01", "1.0.0-prerelease2")).toBe(true);
	});

	test("build metadata is ignored and an unknown version is not older", () => {
		expect(isOlderVersion("1.0.0+5", "1.0.0")).toBe(false);
		expect(isOlderVersion("1.0.0", "1.0.0+5")).toBe(false);
		expect(isOlderVersion("1.0.0-rc.1+5", "1.0.0-rc.1")).toBe(false);
		expect(isOlderVersion("dev", "1.0.0")).toBe(false);
		expect(isOlderVersion("1.0.0", "dev")).toBe(false);
	});
});
