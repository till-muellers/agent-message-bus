import { describe, expect, test } from "bun:test";
import type { Runner } from "../../extension/lib/process/runner.ts";
import { installedPluginVersion, installedSkewWarning } from "../../extension/lib/version/installed-plugin.ts";

/** The output of `omp plugin list --json` on omp 18.12.6 with this plugin installed. */
const LIST_JSON = JSON.stringify({
	npm: [
		{
			name: "@till-muellers/omp-agent-message-bus",
			version: "0.1.0",
			path: "C:\\Users\\tillm\\.omp\\plugins\\node_modules\\@till-muellers\\omp-agent-message-bus",
			manifest: { extensions: ["./extension/index.ts"], version: "0.1.0" },
			enabledFeatures: null,
			enabled: true,
		},
	],
	marketplace: [],
});

/** A runner that answers `stdout` and records the calls. */
function fakeRunner(stdout: string, code = 0): { run: Runner; calls: [string, string[]][] } {
	const calls: [string, string[]][] = [];
	const run: Runner = async (command, args) => {
		calls.push([command, args]);
		return { code, stdout, stderr: "" };
	};
	return { run, calls };
}

describe("installedPluginVersion", () => {
	test("reads the version of the installed plugin from `omp plugin list --json`", async () => {
		const { run, calls } = fakeRunner(LIST_JSON);
		expect(await installedPluginVersion(run)).toBe("0.1.0");
		expect(calls).toEqual([["omp", ["plugin", "list", "--json"]]]);
	});

	test("is unknown when the plugin is not installed", async () => {
		const { run } = fakeRunner(JSON.stringify({ npm: [], marketplace: [] }));
		expect(await installedPluginVersion(run)).toBeUndefined();
	});

	test("is unknown when another plugin with a different name is installed", async () => {
		const { run } = fakeRunner(
			JSON.stringify({ npm: [{ name: "other-plugin", version: "9.9.9" }], marketplace: [] }),
		);
		expect(await installedPluginVersion(run)).toBeUndefined();
	});

	test("is unknown when the command fails", async () => {
		const { run } = fakeRunner("", 1);
		expect(await installedPluginVersion(run)).toBeUndefined();
	});

	test("is unknown when the output is not JSON", async () => {
		const { run } = fakeRunner("not json");
		expect(await installedPluginVersion(run)).toBeUndefined();
	});
});

describe("installedSkewWarning", () => {
	test("warns when the installed plugin is older than the extension protocol", () => {
		expect(installedSkewWarning("0.1.0", "0.2.0")).toBe(
			"the installed omp plugin 0.1.0 is older than the extension protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus",
		);
	});

	test("says nothing when the versions match, when the installed one is newer or when it is unknown", () => {
		expect(installedSkewWarning("0.2.0", "0.2.0")).toBeUndefined();
		expect(installedSkewWarning("0.3.0", "0.2.0")).toBeUndefined();
		expect(installedSkewWarning(undefined, "0.2.0")).toBeUndefined();
	});
});
