/**
 * The version of the omp plugin this machine has installed: `omp plugin list --json` reports the
 * installed copies with their versions (`docs/design/arch/client.md` §8.2).
 */
import type { Runner } from "../process/runner.ts";
import { isOlderVersion, PLUGIN_NAME } from "./plugin-version.ts";

/** One installed plugin of the `npm` list of `omp plugin list --json`. */
interface InstalledPlugin {
	name?: string;
	version?: string;
}

/** One installed plugin of the `marketplace` list of `omp plugin list --json`. */
interface MarketplacePlugin {
	id?: string;
	entries?: { version?: string }[];
}

/** The shape of `omp plugin list --json` (omp 18.12.6). */
interface PluginList {
	npm?: InstalledPlugin[];
	marketplace?: MarketplacePlugin[];
}

/** The version of the plugin with this name; undefined when it is not installed or its version is unknown. */
function versionOf(list: PluginList, name: string): string | undefined {
	const npm = list.npm?.find(plugin => plugin.name === name);
	if (npm?.version) return npm.version;
	const marketplace = list.marketplace?.find(plugin => plugin.id === name);
	const version = marketplace?.entries?.[0]?.version;
	return version || undefined;
}

/** The version of the installed omp plugin; undefined when it cannot be read. */
export async function installedPluginVersion(run: Runner, name: string = PLUGIN_NAME): Promise<string | undefined> {
	const result = await run("omp", ["plugin", "list", "--json"]).catch(() => undefined);
	if (!result || result.code !== 0) return undefined;
	let list: PluginList;
	try {
		list = JSON.parse(result.stdout) as PluginList;
	} catch {
		return undefined;
	}
	return versionOf(list, name);
}

/** The warning when the installed plugin is older than the extension protocol; undefined when it is not. */
export function installedSkewWarning(installed: string | undefined, own: string): string | undefined {
	if (!installed || !isOlderVersion(installed, own)) return undefined;
	return `the installed omp plugin ${installed} is older than the extension protocol ${own}; run omp plugin upgrade ${PLUGIN_NAME}`;
}
