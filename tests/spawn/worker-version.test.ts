import { describe, expect, test } from "bun:test";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { skewError, workerPluginVersion } from "../../extension/lib/spawn/worker-version.ts";
import { useTempDirs } from "../fakes.ts";

describe("workerPluginVersion", () => {
	const tempDir = useTempDirs();

	/** A plugin directory with an extension file and the package.json two levels above it. */
	async function plugin(version: string): Promise<string> {
		const dir = await tempDir();
		await mkdir(join(dir, "extension"), { recursive: true });
		await writeFile(join(dir, "extension", "index.ts"), "export default () => {};\n");
		await writeFile(join(dir, "package.json"), JSON.stringify({ name: "worker-plugin", version }));
		return join(dir, "extension", "index.ts");
	}

	test("the forwarded extension decides the version, not the installed plugin", async () => {
		const forwarded = await plugin("0.2.0");
		expect(await workerPluginVersion({ forwardedExtension: forwarded, installed: async () => "0.1.0" })).toBe("0.2.0");
	});

	test("without a forwarded extension the installed plugin decides", async () => {
		expect(await workerPluginVersion({ installed: async () => "0.1.0" })).toBe("0.1.0");
	});

	test("is unknown when the forwarded extension has no package.json", async () => {
		const dir = await tempDir();
		const forwarded = join(dir, "index.ts");
		await writeFile(forwarded, "export default () => {};\n");
		expect(await workerPluginVersion({ forwardedExtension: forwarded, installed: async () => "0.1.0" })).toBeUndefined();
	});

	test("is unknown when neither the forwarded extension nor the installed plugin can be read", async () => {
		expect(await workerPluginVersion({ installed: async () => undefined })).toBeUndefined();
	});
});

describe("skewError", () => {
	test("an older worker plugin is refused with the upgrade command", () => {
		expect(skewError("0.1.0", "0.2.0")).toBe(
			"the worker would load omp plugin 0.1.0, which is older than the protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus",
		);
	});

	test("the same or a newer worker plugin proceeds", () => {
		expect(skewError("0.2.0", "0.2.0")).toBeUndefined();
		expect(skewError("0.3.0", "0.2.0")).toBeUndefined();
	});

	test("an unknown worker plugin version proceeds", () => {
		expect(skewError(undefined, "0.2.0")).toBeUndefined();
	});
});
