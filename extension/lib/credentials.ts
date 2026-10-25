/**
 * Agent-bus credential storage.
 *
 * `/bus login` stores the server url plus an admin and/or agent token in the
 * active omp agent directory (`PI_CODING_AGENT_DIR`, default `~/.omp/agent`)
 * under `agent-message-bus/credentials.json`, owner-only (0600; Windows ignores
 * the mode and relies on the user-profile ACL). `OMP_AGENT_BUS_DIR` overrides
 * the directory. `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` in the environment override
 * the stored url/token, so spawned agents can run on member tokens.
 */
import { chmod, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

export interface StoredCredentials {
	url: string;
	adminToken?: string;
	token?: string;
}

/** The url and agent token the session acts with. */
export interface ActiveCredentials {
	url: string;
	token: string;
}

export interface CredentialStore {
	/** Absolute path of the credential file (it may not exist). */
	readonly path: string;
	load(): Promise<StoredCredentials | undefined>;
	save(credentials: StoredCredentials): Promise<void>;
	/** Deletes the credential file; resolves false when there was none. */
	clear(): Promise<boolean>;
}

export function configDir(env: NodeJS.ProcessEnv = process.env): string {
	if (env.OMP_AGENT_BUS_DIR) return env.OMP_AGENT_BUS_DIR;
	return join(env.PI_CODING_AGENT_DIR || join(homedir(), ".omp", "agent"), "agent-message-bus");
}

export function trimUrl(url: string): string {
	return url.replace(/\/+$/, "");
}

function isMissing(error: unknown): boolean {
	return (error as NodeJS.ErrnoException)?.code === "ENOENT";
}

function optionalString(value: unknown): string | undefined {
	return typeof value === "string" && value ? value : undefined;
}

export function createCredentialStore(env: NodeJS.ProcessEnv = process.env, dir: string = configDir(env)): CredentialStore {
	const path = join(dir, "credentials.json");
	return {
		path,
		async load() {
			let raw: string;
			try {
				raw = await readFile(path, "utf8");
			} catch (error) {
				if (isMissing(error)) return undefined;
				throw error;
			}
			try {
				const parsed = JSON.parse(raw) as Partial<Record<keyof StoredCredentials, unknown>>;
				if (typeof parsed.url !== "string" || !parsed.url) throw new Error("no url");
				const credentials: StoredCredentials = { url: parsed.url };
				const adminToken = optionalString(parsed.adminToken);
				const token = optionalString(parsed.token);
				if (adminToken) credentials.adminToken = adminToken;
				if (token) credentials.token = token;
				return credentials;
			} catch {
				throw new Error(`Credential file ${path} is unreadable. Run /bus login to rewrite it.`);
			}
		},
		async save(credentials) {
			await mkdir(dir, { recursive: true });
			await writeFile(path, `${JSON.stringify(credentials, null, 2)}\n`, { mode: 0o600 });
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

/** Env `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` (both set) win; else the stored agent token. Admin-only → undefined. */
export function resolveActive(env: NodeJS.ProcessEnv, stored: StoredCredentials | undefined): ActiveCredentials | undefined {
	if (env.AGENT_BUS_URL && env.AGENT_BUS_TOKEN) return { url: trimUrl(env.AGENT_BUS_URL), token: env.AGENT_BUS_TOKEN };
	if (stored?.token) return { url: trimUrl(stored.url), token: stored.token };
	return undefined;
}
