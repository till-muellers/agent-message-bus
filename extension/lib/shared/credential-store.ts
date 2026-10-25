/**
 * Owner-only JSON file store for extension credentials.
 *
 * `_shared/` is a support library folder, NOT an extension: it has no
 * `index.ts`, so omp never auto-loads it. Extension packages import it
 * explicitly and supply the file location, the content validation and the
 * error to show when the file cannot be parsed.
 *
 * Files are written with mode 0600 and an existing file is chmod-ed to 0600
 * (Windows ignores the mode and relies on the user-profile ACL).
 */
import { chmod, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

export interface JsonFileStore<T> {
	/** Absolute path of the file (it may not exist). */
	readonly path: string;
	/** Resolves undefined when the file does not exist. */
	load(): Promise<T | undefined>;
	save(value: T): Promise<void>;
	/** Deletes the file; resolves false when there was none. */
	clear(): Promise<boolean>;
}

export interface JsonFileStoreOptions<T> {
	dir: string;
	file: string;
	/** Validates the parsed JSON; throws when it is not a `T`. */
	parse(json: unknown): T;
	/** The error thrown when the file exists but `JSON.parse` or `parse` fails. */
	unreadable(path: string): Error;
}

/** The active omp agent directory: `PI_CODING_AGENT_DIR`, default `~/.omp/agent`. */
export function agentConfigDir(env: NodeJS.ProcessEnv = process.env): string {
	return env.PI_CODING_AGENT_DIR || join(homedir(), ".omp", "agent");
}

function isMissing(error: unknown): boolean {
	return (error as NodeJS.ErrnoException | undefined)?.code === "ENOENT";
}

async function readOptional(path: string): Promise<string | undefined> {
	try {
		return await readFile(path, "utf8");
	} catch (error) {
		if (isMissing(error)) return undefined;
		throw error;
	}
}

export function createJsonFileStore<T>({ dir, file, parse, unreadable }: JsonFileStoreOptions<T>): JsonFileStore<T> {
	const path = join(dir, file);
	return {
		path,
		async load() {
			const raw = await readOptional(path);
			if (raw === undefined) return undefined;
			try {
				return parse(JSON.parse(raw));
			} catch {
				throw unreadable(path);
			}
		},
		async save(value) {
			await mkdir(dir, { recursive: true });
			await writeFile(path, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 });
			// writeFile's mode only applies to new files; tighten an existing one too.
			await chmod(path, 0o600).catch(() => {});
		},
		async clear() {
			try {
				await rm(path);
				return true;
			} catch (error) {
				if (isMissing(error)) return false;
				throw error;
			}
		},
	};
}
