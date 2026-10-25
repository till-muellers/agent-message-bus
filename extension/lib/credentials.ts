/**
 * Agent-bus credential storage.
 *
 * `/bus login` stores the server url plus an admin and/or agent token in the
 * active omp agent directory (`PI_CODING_AGENT_DIR`, default `~/.omp/agent`)
 * under `agent-message-bus/credentials.json`, owner-only (0600; Windows ignores
 * the mode and relies on the user-profile ACL). `OMP_AGENT_BUS_DIR` overrides
 * the directory. `AGENT_BUS_URL` + `AGENT_BUS_TOKEN` are read once at load
 * (`lib/env.ts`) into the session and override the stored url/token, so
 * other clients can hand a session a token directly. A member token from `/bus accept`
 * is kept per omp session (`lib/session-identity.ts`) and loaded into
 * `SessionCredentials.accepted`; env credentials win over it. A leader token
 * from `/bus promote` lives only in the session (`SessionCredentials`) and
 * overrides all of them; it is never written to a file. `/bus logout` clears
 * them all.
 */
import { join } from "node:path";
import { agentConfigDir, createJsonFileStore, type JsonFileStore } from "./shared/credential-store.ts";

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

/**
 * Credentials held by the session: the leader token minted by `/bus promote`, the env credentials read at load and
 * the member token accepted by `/bus accept` (owned and persisted per session by `lib/session-identity.ts`).
 */
export interface SessionCredentials {
	override?: ActiveCredentials;
	env?: ActiveCredentials;
	accepted?: ActiveCredentials;
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

/** Precedence: session override (promote) > env credentials (read at load) > accepted invite > stored agent token. Admin-only → undefined. */
export function resolveActive(session: SessionCredentials, stored: StoredCredentials | undefined): ActiveCredentials | undefined {
	if (session.override) return session.override;
	if (session.env) return session.env;
	if (session.accepted) return session.accepted;
	if (stored?.token) return { url: trimUrl(stored.url), token: stored.token };
	return undefined;
}
