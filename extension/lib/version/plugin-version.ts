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

/** A version split into its numeric core and its pre-release identifiers (build metadata is dropped). */
interface ParsedVersion {
	core: number[];
	pre: string[];
}

/** The numeric parts of a version core; undefined when a part does not start with a number. */
function parts(core: string): number[] | undefined {
	const numbers: number[] = [];
	for (const chunk of core.split(".")) {
		const value = Number.parseInt(chunk, 10);
		if (Number.isNaN(value)) return undefined;
		numbers.push(value);
	}
	return numbers;
}

/** `1.2.3-rc.1+5` as core `[1, 2, 3]` and pre-release `["rc", "1"]`; undefined when the core is not numeric. */
function parse(version: string): ParsedVersion | undefined {
	const withoutBuild = version.split("+", 1)[0] ?? "";
	const dash = withoutBuild.indexOf("-");
	const core = parts(dash < 0 ? withoutBuild : withoutBuild.slice(0, dash));
	if (!core) return undefined;
	return { core, pre: dash < 0 ? [] : withoutBuild.slice(dash + 1).split(".") };
}

/** SemVer 2.0.0 precedence of two pre-release identifiers: numeric ones numerically and below alphanumeric ones, the rest in ASCII order. */
function compareIdentifiers(a: string, b: string): number {
	const aNumeric = /^[0-9]+$/.test(a);
	const bNumeric = /^[0-9]+$/.test(b);
	if (aNumeric && bNumeric) return Number(a) - Number(b);
	if (aNumeric !== bNumeric) return aNumeric ? -1 : 1;
	return a < b ? -1 : a > b ? 1 : 0;
}

/** Whether the pre-release identifiers `left` rank below `right`; with equal leading identifiers the shorter list ranks below. */
function preReleaseBelow(left: string[], right: string[]): boolean {
	for (let i = 0; i < Math.min(left.length, right.length); i++) {
		const order = compareIdentifiers(left[i] ?? "", right[i] ?? "");
		if (order !== 0) return order < 0;
	}
	return left.length < right.length;
}

/**
 * Whether `version` is older than `other` by SemVer 2.0.0 precedence: the numeric core first (a missing part counts as 0),
 * then a pre-release ranks below its release, build metadata is ignored. An unknown version is not older.
 */
export function isOlderVersion(version: string, other: string): boolean {
	const left = parse(version);
	const right = parse(other);
	if (!left || !right) return false;
	for (let i = 0; i < Math.max(left.core.length, right.core.length); i++) {
		const a = left.core[i] ?? 0;
		const b = right.core[i] ?? 0;
		if (a !== b) return a < b;
	}
	if (left.pre.length === 0) return false;
	return right.pre.length === 0 || preReleaseBelow(left.pre, right.pre);
}
