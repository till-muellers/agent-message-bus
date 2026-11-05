/**
 * The connection's `bus_*` tools: registered once per session behind the server-protocol gate, then switched on and
 * off with the connection (`activation.ts`).
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { BusClient } from "../bus/client.ts";
import type { ActiveCredentials } from "../bus/credentials.ts";
import { registerRemoteTools } from "../tools/remote-tools.ts";
import { activateBusTools, deactivateTools } from "./activation.ts";
import type { ConnectionInbox } from "./connection-inbox.ts";
import type { ServerGate } from "./server-gate.ts";
import type { SessionView } from "./session-view.ts";

export interface ConnectionToolsDeps {
	pi: ExtensionAPI;
	client: BusClient;
	view: SessionView;
	/** Keeps the extension off a server that is too old; checked before the tools are registered. */
	gate: ServerGate;
	warn(message: string): void;
	/** Tools this extension registers itself (`bus_spawn_worker`); they are switched on and off with the server tools. */
	localTools: readonly string[];
	/** Awaited after a successful membership tool (the `refreshesMembership` rows of `tools/tool-policy.ts`). */
	membershipChanged?: () => Promise<void>;
	/** The bus agent the session acts as, for the outgoing cards of the send tools. */
	self?: () => string | undefined;
}

export interface ConnectionTools {
	/** Registers the session's tools unless the server is too old; false when it is. */
	connect(creds: ActiveCredentials): Promise<boolean>;
	/** Switches the session's tools on. */
	activate(): Promise<void>;
	/** Switches exactly the session's tools off. */
	deactivate(): Promise<void>;
}

export function createConnectionTools(
	deps: ConnectionToolsDeps,
	getActive: () => ActiveCredentials | undefined,
	inbox: Pick<ConnectionInbox, "wait" | "threadRead">,
): ConnectionTools {
	const { pi, client, view, warn } = deps;
	const state = { registered: false, names: [] as string[] };
	const ours = () => [...state.names, ...deps.localTools];
	return {
		async connect(creds) {
			const verdict = await deps.gate.check(creds.url);
			if (verdict !== "ok") {
				warn(verdict.refused);
				return false;
			}
			const kind = view.sub ? "sub" : "main";
			const hooks = { threadRead: inbox.threadRead, membershipChanged: deps.membershipChanged, self: deps.self };
			await registerRemoteTools(pi, { client, getActive, state, warn, kind, agentId: view.agentId, wait: inbox.wait, ...hooks });
			return true;
		},
		activate: () => activateBusTools(pi, ours()),
		deactivate: () => deactivateTools(pi, ours()),
	};
}
