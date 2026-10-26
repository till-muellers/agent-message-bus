/**
 * Bus settings taken from the environment once, at extension load:
 * `AGENT_BUS_URL`, `AGENT_BUS_TOKEN` and `AGENT_BUS_BIN` are copied into
 * memory and deleted from the environment so child processes and subagents
 * do not inherit them.
 */
import type { ActiveCredentials } from "./stored-credentials.ts";
import { trimUrl } from "./stored-credentials.ts";

export interface BusEnv {
	/** `AGENT_BUS_URL` + `AGENT_BUS_TOKEN`, when both were set. */
	credentials?: ActiveCredentials;
	/** `AGENT_BUS_BIN`: the agent-message-bus binary to run for `/bus server`. */
	bin?: string;
}

const KEYS = ["AGENT_BUS_URL", "AGENT_BUS_TOKEN", "AGENT_BUS_BIN"] as const;

/** Reads the bus variables and deletes them from `env`. */
export function consumeEnv(env: NodeJS.ProcessEnv): BusEnv {
	const [url, token, bin] = KEYS.map(key => env[key]);
	for (const key of KEYS) delete env[key];
	const result: BusEnv = {};
	if (url && token) result.credentials = { url: trimUrl(url), token };
	if (bin) result.bin = bin;
	return result;
}
