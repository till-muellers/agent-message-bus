/**
 * The plugin version the worker will load: the forwarded extension's own package.json decides, else
 * the installed omp plugin; an unknown version proceeds (`docs/design/arch/client.md` §8.2).
 */
import { dirname, resolve } from "node:path";
import { isOlderVersion, PLUGIN_NAME, pluginVersion } from "../version/plugin-version.ts";

export interface WorkerVersionDeps {
	/** The extension the leader forwards to the worker; its package.json is two levels up from it. */
	forwardedExtension?: string;
	/** The version of the installed omp plugin. */
	installed(): Promise<string | undefined>;
}

/** The package.json of a forwarded extension: two levels up from `extension/index.ts`. */
function forwardedPackageJson(extension: string): string {
	return resolve(dirname(extension), "..", "package.json");
}

/** The version the worker will load; undefined when it cannot be read. */
export async function workerPluginVersion(deps: WorkerVersionDeps): Promise<string | undefined> {
	// A forwarded extension decides; it is the one the worker loads.
	if (deps.forwardedExtension) return pluginVersion(forwardedPackageJson(deps.forwardedExtension)).catch(() => undefined);
	return deps.installed();
}

/** The error when the worker would load an older plugin than this one implements; undefined when it may spawn. */
export function skewError(worker: string | undefined, own: string): string | undefined {
	if (!worker || !isOlderVersion(worker, own)) return undefined;
	return `the worker would load omp plugin ${worker}, which is older than the protocol ${own}; run omp plugin upgrade ${PLUGIN_NAME}`;
}
