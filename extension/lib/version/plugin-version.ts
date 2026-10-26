/**
 * This plugin's own version: the `version` of the package.json that ships it. Every release bumps it,
 * so it is the single source of truth for the protocol version the extension implements
 * (`docs/design/arch/client.md` §8.2).
 */
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

/** The name omp knows this plugin by; the upgrade command names it. */
export const PLUGIN_NAME = "@till-muellers/omp-agent-message-bus";

/** The package.json of this plugin: three levels up from `lib/version/`. */
export const PACKAGE_JSON = resolve(import.meta.dir, "..", "..", "..", "package.json");

/** The version of the given package.json; rejects when it holds none. */
export async function pluginVersion(packageJsonPath: string = PACKAGE_JSON): Promise<string> {
	const parsed = JSON.parse(await readFile(packageJsonPath, "utf8")) as { version?: unknown };
	if (typeof parsed.version !== "string" || !parsed.version) throw new Error(`no version in ${packageJsonPath}`);
	return parsed.version;
}

/** The numeric parts of a version; undefined when it does not start with a number. */
function parts(version: string): number[] | undefined {
	const numbers: number[] = [];
	for (const chunk of version.split(".")) {
		const value = Number.parseInt(chunk, 10);
		if (Number.isNaN(value)) return undefined;
		numbers.push(value);
	}
	return numbers;
}

/** Whether `version` is older than `other`; a prerelease suffix is ignored, an unknown version is not older. */
export function isOlderVersion(version: string, other: string): boolean {
	const left = parts(version);
	const right = parts(other);
	if (!left || !right) return false;
	for (let i = 0; i < Math.max(left.length, right.length); i++) {
		const a = left[i] ?? 0;
		const b = right[i] ?? 0;
		if (a !== b) return a < b;
	}
	return false;
}
