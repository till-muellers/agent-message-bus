/**
 * What the session tells the model about the bus (`docs/design/arch/client.md` §5): the cached identity and, in a
 * main session, the guide, rendered as the agent-bus-context block. Both are read when the connection restarts;
 * the identity also after a membership tool. `block()` only reads the cache.
 */
import type { BusClient } from "../bus/client.ts";
import { NOT_LOGGED_IN } from "../bus/errors.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import { mainBlock, subagentBlock } from "./context-block.ts";
import { createGuide } from "./guide.ts";
import { createIdentityCache } from "./identity-cache.ts";

export interface BusContext {
	/** Connection restart: the identity and, in a main session, the guide are read again. */
	refresh(): Promise<void>;
	/** A membership tool succeeded: the identity is read again. */
	refreshIdentity(): Promise<void>;
	clear(): void;
	/** The block text; undefined while the session has no bus identity. */
	block(): string | undefined;
	/** The agent the session acts as and, for a member, its space; empty while unknown. */
	who(): { agent?: string; space?: string };
}

export interface BusContextDeps {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
	warn(message: string): void;
	/** Whether the session is a subagent, and its agent id. */
	isSub(): boolean;
	agentId(): string | undefined;
}

export function createBusContext(deps: BusContextDeps): BusContext {
	const { client, getActive } = deps;
	const identity = createIdentityCache({ client, getActive });
	const guide = createGuide(async () => {
		const active = getActive();
		if (!active) throw new Error(NOT_LOGGED_IN);
		return client.fetchGuide(active.url, active.token);
	}, deps.warn);
	return {
		async refresh() {
			const loadGuide = !deps.isSub() && getActive() !== undefined;
			await Promise.all([identity.refresh(), loadGuide ? guide.load() : undefined]);
		},
		refreshIdentity: identity.refresh,
		clear: identity.clear,
		block() {
			const id = identity.current();
			if (!id) return undefined;
			return deps.isSub() ? subagentBlock(id, deps.agentId() ?? "<id>") : mainBlock(id, guide.text());
		},
		who() {
			const id = identity.current();
			return id ? { agent: id.name, space: id.space } : {};
		},
	};
}
