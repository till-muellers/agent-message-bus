/**
 * Downloads the latest agent-message-bus release binary for this platform from
 * GitHub, verifies it against the release's SHA256SUMS and caches it under
 * `<dir>/<version>/`.
 */
import { createHash } from "node:crypto";
import { access, chmod, mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { FetchLike } from "./client.ts";

export const RELEASES_URL = "https://api.github.com/repos/till-muellers/agent-message-bus/releases/latest";

const OS: Record<string, string> = { win32: "windows", linux: "linux", darwin: "darwin" };
const ARCH: Record<string, string> = { x64: "amd64", arm64: "arm64" };

export interface DownloadOptions {
	fetchFn: FetchLike;
	/** Cache root: `<agentConfigDir>/agent-message-bus/bin`. */
	dir: string;
	platform: string;
	arch: string;
}

interface Release {
	tag_name: string;
	assets: { name: string; browser_download_url: string }[];
}

/** `agent-message-bus` plus `.exe` on Windows. */
export function binaryName(platform: string): string {
	return platform === "win32" ? "agent-message-bus.exe" : "agent-message-bus";
}

/** The release asset name for `version` on this platform; throws when the platform has no release build. */
export function assetName(version: string, platform: string, arch: string): string {
	const os = OS[platform];
	const goarch = ARCH[arch];
	if (!os || !goarch) throw new Error(`no agent-message-bus release for ${platform}/${arch}`);
	return `agent-message-bus_${version}_${os}_${goarch}${platform === "win32" ? ".exe" : ""}`;
}

async function get(fetchFn: FetchLike, url: string, accept = "application/octet-stream"): Promise<Response> {
	const response = await fetchFn(url, { method: "GET", headers: { Accept: accept, "User-Agent": "omp-agent-message-bus" } });
	if (!response.ok) throw new Error(`download ${url} failed: HTTP ${response.status}`);
	return response;
}

async function latestRelease(fetchFn: FetchLike): Promise<Release> {
	const release = (await (await get(fetchFn, RELEASES_URL, "application/vnd.github+json")).json()) as Partial<Release>;
	if (typeof release.tag_name !== "string" || !Array.isArray(release.assets)) throw new Error("unexpected GitHub release response");
	return { tag_name: release.tag_name, assets: release.assets };
}

function assetUrl(release: Release, name: string): string {
	const asset = release.assets.find(a => a.name === name);
	if (!asset) throw new Error(`release ${release.tag_name} has no asset ${name}`);
	return asset.browser_download_url;
}

/** The expected sha256 of `name` from a `SHA256SUMS` body (`<hex>  <file>` lines). */
export function expectedSum(sums: string, name: string): string {
	for (const line of sums.split(/\r?\n/)) {
		const [hash, file] = line.trim().split(/\s+\*?/);
		if (file === name && hash) return hash.toLowerCase();
	}
	throw new Error(`SHA256SUMS has no entry for ${name}`);
}

async function exists(path: string): Promise<boolean> {
	return access(path).then(
		() => true,
		() => false,
	);
}

async function fetchVerified(fetchFn: FetchLike, release: Release, name: string): Promise<Uint8Array> {
	const sums = await (await get(fetchFn, assetUrl(release, "SHA256SUMS"))).text();
	const bytes = new Uint8Array(await (await get(fetchFn, assetUrl(release, name))).arrayBuffer());
	const actual = createHash("sha256").update(bytes).digest("hex");
	if (actual !== expectedSum(sums, name)) throw new Error(`checksum mismatch for ${name}; download rejected`);
	return bytes;
}

/** Returns the cached binary of the latest release, downloading and verifying it first when missing. */
export async function downloadBinary(options: DownloadOptions): Promise<string> {
	const release = await latestRelease(options.fetchFn);
	const version = release.tag_name.replace(/^v/, "");
	const name = assetName(version, options.platform, options.arch);
	const target = join(options.dir, version, binaryName(options.platform));
	if (await exists(target)) return target;
	const bytes = await fetchVerified(options.fetchFn, release, name);
	await mkdir(join(options.dir, version), { recursive: true });
	await writeFile(target, bytes);
	await chmod(target, 0o755);
	return target;
}
