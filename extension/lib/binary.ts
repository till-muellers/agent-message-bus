/**
 * Resolves the agent-message-bus binary for `/bus server`: `AGENT_BUS_BIN`
 * (read once at load), else `agent-message-bus` on PATH, else the verified
 * download of the latest release (`lib/download.ts`).
 */
import { access, constants } from "node:fs/promises";
import { delimiter, join } from "node:path";
import { binaryName, type DownloadOptions, downloadBinary } from "./download.ts";

export interface BinaryOptions extends DownloadOptions {
	/** `AGENT_BUS_BIN` from the environment. */
	bin?: string;
	/** Finds an executable on PATH; undefined when absent. */
	which(name: string): Promise<string | undefined>;
}

/** Searches the directories of `pathVar` for an executable `name`. */
export async function findOnPath(name: string, pathVar: string | undefined): Promise<string | undefined> {
	for (const dir of (pathVar ?? "").split(delimiter).filter(Boolean)) {
		const candidate = join(dir, name);
		const found = await access(candidate, constants.X_OK).then(
			() => true,
			() => false,
		);
		if (found) return candidate;
	}
	return undefined;
}

export async function resolveBinary(options: BinaryOptions): Promise<string> {
	if (options.bin) return options.bin;
	const onPath = await options.which(binaryName(options.platform));
	return onPath ?? (await downloadBinary(options));
}
