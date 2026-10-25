/**
 * Agent-bus credential storage.
 *
 * `/bus login` stores the server url plus an admin and/or agent token in the
 * active omp agent directory (`PI_CODING_AGENT_DIR`, default `~/.omp/agent`)
 * under `agent-message-bus/credentials.json`, owner-only (0600; Windows ignores
 * the mode and relies on the user-profile ACL). `OMP_AGENT_BUS_DIR` overrides
 * the directory. `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` in the environment override
 * the stored url/token, so spawned agents can run on member tokens. A leader
 * token from `/bus promote` lives only in the session (`SessionCredentials`) and
 * overrides both; it is never written to the file.
 */
import { join } from "node:path";
import { agentConfigDir, createJsonFileStore, type JsonFileStore } from "../../_shared/credential-store.ts";

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

/** Per-session credentials held in memory only: the leader token minted by `/bus promote`. */
export interface SessionCredentials {
	override?: ActiveCredentials;
}

export type CredentialStore = JsonFileStore<StoredCredentials>;

export function configDir(env: NodeJS.ProcessEnv = process.env): string {
	return env.OMP_AGENT_BUS_DIR || join(agentConfigDir(env), "agent-message-bus");
}

export function trimUrl(url: string): string {
	return url.replace(/\/+$/, "");
}

function optionalString(value: unknown): string | undefined {
	return typeof value === "string" && value ? value : undefined;
}

function parseCredentials(json: unknown): StoredCredentials {
	const parsed = json as Partial<Record<keyof StoredCredentials, unknown>> | null;
	if (typeof parsed?.url !== "string" || !parsed.url) throw new Error("no url");
	const credentials: StoredCredentials = { url: parsed.url };
	const adminToken = optionalString(parsed.adminToken);
	const token = optionalString(parsed.token);
	if (adminToken) credentials.adminToken = adminToken;
	if (token) credentials.token = token;
	return credentials;
}

export function createCredentialStore(env: NodeJS.ProcessEnv = process.env, dir: string = configDir(env)): CredentialStore {
	return createJsonFileStore({
		dir,
		file: "credentials.json",
		parse: parseCredentials,
		unreadable: path => new Error(`Credential file ${path} is unreadable. Run /bus login to rewrite it.`),
	});
}

/** Precedence: session override (promote) > env `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` (both set) > stored agent token. Admin-only → undefined. */
export function resolveActive(
	session: SessionCredentials,
	env: NodeJS.ProcessEnv,
	stored: StoredCredentials | undefined,
): ActiveCredentials | undefined {
	if (session.override) return session.override;
	if (env.AGENT_BUS_URL && env.AGENT_BUS_TOKEN) return { url: trimUrl(env.AGENT_BUS_URL), token: env.AGENT_BUS_TOKEN };
	if (stored?.token) return { url: trimUrl(stored.url), token: stored.token };
	return undefined;
}
