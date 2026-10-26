import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readFile, stat } from "node:fs/promises";
import { join } from "node:path";
import { activateBusTools, deactivateBusTools, deactivateTools } from "../extension/lib/connection/activation.ts";
import { resolveBinary } from "../extension/lib/binary.ts";
import type { FetchLike } from "../extension/lib/bus/client.ts";
import { assetName, expectedSum, RELEASES_URL } from "../extension/lib/download.ts";
import type { Runner } from "../extension/lib/runner.ts";
import { createServerCli } from "../extension/lib/server.ts";
import { useTempDirs } from "./fakes.ts";

describe("activation", () => {
	test("deactivate drops every bus_* tool; activate re-adds the missing names only", async () => {
		let active = ["read", "bus_a", "edit", "bus_b"];
		const pi = { getActiveTools: () => [...active], setActiveTools: async (names: string[]) => void (active = names) };
		await deactivateBusTools(pi);
		expect(active).toEqual(["read", "edit"]);
		await activateBusTools(pi, ["bus_a", "bus_b"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
		await activateBusTools(pi, ["bus_a"]);
		expect(active).toEqual(["read", "edit", "bus_a", "bus_b"]);
	});

	test("deactivateTools drops exactly the given names", async () => {
		let active = ["read", "bus_a", "bus_foreign", "bus_b"];
		const pi = { getActiveTools: () => [...active], setActiveTools: async (names: string[]) => void (active = names) };
		await deactivateTools(pi, ["bus_a", "bus_b", "bus_missing"]);
		expect(active).toEqual(["read", "bus_foreign"]);
	});
});

describe("server cli", () => {
	test("runs the resolved binary and returns stdout; a non-zero exit is an error with stderr", async () => {
		const runner: Runner = async (_command, args) => (args[0] === "ok" ? { code: 0, stdout: "out", stderr: "" } : { code: 1, stdout: "", stderr: "boom\n" });
		const cli = createServerCli(runner, async () => "/bin/amb");
		expect(await cli(["ok"])).toBe("out");
		await expect(cli(["stop"])).rejects.toThrow("agent-message-bus stop failed (exit 1): boom");
	});
});

describe("binary resolution", () => {
	const tempDir = useTempDirs();
	const BYTES = "fake-binary";
	const sha = (text: string) => createHash("sha256").update(text).digest("hex");
	const ASSET = "agent-message-bus_1.2.3_linux_amd64";

	/** Fake GitHub: latest release v1.2.3 with the linux/amd64 asset and SHA256SUMS listing `sum`. */
	function fakeGitHub(sum = sha(BYTES)) {
		const urls: string[] = [];
		const fetchFn: FetchLike = async url => {
			urls.push(url);
			if (url === RELEASES_URL)
				return Response.json({
					tag_name: "v1.2.3",
					assets: [
						{ name: ASSET, browser_download_url: "https://dl/asset" },
						{ name: "SHA256SUMS", browser_download_url: "https://dl/sums" },
					],
				});
			if (url === "https://dl/sums") return new Response(`${sha("other")}  agent-message-bus_1.2.3_darwin_arm64\n${sum}  ${ASSET}\n`);
			if (url === "https://dl/asset") return new Response(BYTES);
			return new Response("missing", { status: 404 });
		};
		return { urls, fetchFn };
	}

	async function options(fetchFn: FetchLike, extra: { bin?: string; onPath?: string } = {}) {
		return { bin: extra.bin, which: async () => extra.onPath, fetchFn, dir: await tempDir(), platform: "linux", arch: "x64" };
	}

	test("AGENT_BUS_BIN wins over PATH; PATH wins over a download", async () => {
		const gh = fakeGitHub();
		expect(await resolveBinary(await options(gh.fetchFn, { bin: "/env/amb", onPath: "/usr/bin/agent-message-bus" }))).toBe("/env/amb");
		expect(await resolveBinary(await options(gh.fetchFn, { onPath: "/usr/bin/agent-message-bus" }))).toBe("/usr/bin/agent-message-bus");
		expect(gh.urls).toEqual([]);
	});

	test("downloads the latest release asset, verifies its sha256, caches it under <dir>/<version>/ and reuses the cache", async () => {
		const gh = fakeGitHub();
		const opts = await options(gh.fetchFn);
		const path = await resolveBinary(opts);
		expect(path).toBe(join(opts.dir, "1.2.3", "agent-message-bus"));
		expect(await readFile(path, "utf8")).toBe(BYTES);
		if (process.platform !== "win32") expect((await stat(path)).mode & 0o777).toBe(0o755);
		expect(await resolveBinary(opts)).toBe(path);
		expect(gh.urls).toEqual([RELEASES_URL, "https://dl/sums", "https://dl/asset", RELEASES_URL]);
	});

	test("a checksum mismatch is an error and nothing is cached", async () => {
		const opts = await options(fakeGitHub(sha("tampered")).fetchFn);
		await expect(resolveBinary(opts)).rejects.toThrow(`checksum mismatch for ${ASSET}`);
		await expect(stat(join(opts.dir, "1.2.3"))).rejects.toThrow();
	});

	test("a failed release lookup is an error", async () => {
		const opts = await options(async () => new Response("rate limited", { status: 403 }));
		await expect(resolveBinary(opts)).rejects.toThrow(`download ${RELEASES_URL} failed: HTTP 403`);
	});

	test("assetName maps platform and arch to the release naming; unsupported platforms are an error", () => {
		for (const [platform, arch, name] of [
			["win32", "x64", "agent-message-bus_1.0.0_windows_amd64.exe"],
			["linux", "arm64", "agent-message-bus_1.0.0_linux_arm64"],
			["darwin", "x64", "agent-message-bus_1.0.0_darwin_amd64"],
		] as const)
			expect(assetName("1.0.0", platform, arch)).toBe(name);
		expect(() => assetName("1.0.0", "aix", "ppc64")).toThrow("no agent-message-bus release for aix/ppc64");
	});

	test("expectedSum reads text and binary-mode SHA256SUMS lines; a missing entry is an error", () => {
		expect(expectedSum("ABC  a\ndef *b\n", "a")).toBe("abc");
		expect(expectedSum("ABC  a\ndef *b\n", "b")).toBe("def");
		expect(() => expectedSum("abc  a\n", "c")).toThrow("SHA256SUMS has no entry for c");
	});
});
