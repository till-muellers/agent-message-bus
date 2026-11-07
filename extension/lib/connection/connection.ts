/**
 * The session's live bus connection: the active credentials (`connection-identity.ts`), the registered `bus_*` tools
 * (`connection-tools.ts`) and the inbox (`connection-inbox.ts`). A main session publishes the credentials it acts
 * with for its subagents; a subagent acts only with what its root session published, registers no inbox tools and
 * runs no inbox loop: its main session owns the inbox.
 */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { ActiveCredentials } from "../bus/credentials.ts";
import type { InboxLedger } from "../delivery/inbox-ledger.ts";
import { deactivateBusTools } from "./activation.ts";
import { type ConnectionIdentityDeps, createConnectionIdentity } from "./connection-identity.ts";
import { type ConnectionInboxDeps, createConnectionInbox } from "./connection-inbox.ts";
import { type ConnectionToolsDeps, createConnectionTools } from "./connection-tools.ts";

export interface ConnectionDeps extends ConnectionIdentityDeps, ConnectionInboxDeps, ConnectionToolsDeps {
	pi: ExtensionAPI;
	/** Awaited on every restart once it is known whether the server was connected, before the inbox stream opens. */
	connected?: (connected: boolean) => Promise<void>;
}

export interface Connection {
	getActive(): ActiveCredentials | undefined;
	/** The inbox ledger of the current connection start; it knows what this start handed over. */
	ledger(): InboxLedger;
	/** Marks entries consumed in the current start's ledger; the ack follows it. */
	consume(ids: readonly number[]): void;
	/** A new connection start: re-resolves the credentials, probes the server again, registers the server tools once, switches the tools on or off to match and, once they are active, restarts the inbox loop. */
	restart(): Promise<void>;
	/** Stops the loop, drops the credentials and deactivates every bus tool; registered tools stay and fail until the next login. */
	logout(): Promise<void>;
	/** Session end: stops the loop and withdraws the identity published for the subagents. */
	stop(): void;
}

export function createConnection(deps: ConnectionDeps): Connection {
	const identity = createConnectionIdentity(deps);
	const inbox = createConnectionInbox(deps, identity.getActive);
	const tools = createConnectionTools(deps, identity.getActive, inbox);
	return {
		getActive: identity.getActive,
		ledger: inbox.ledger,
		consume: inbox.consume,
		stop() {
			inbox.stop();
			identity.withdraw();
		},
		async restart() {
			inbox.startOver();
			deps.gate.startOver();
			const creds = await identity.resolve();
			const connected = creds !== undefined && (await tools.connect(creds));
			// The context block must be ready and the tools active before a streamed entry can start a turn.
			await deps.connected?.(connected);
			if (!connected) return tools.deactivate();
			await tools.activate();
			if (!deps.view.sub) inbox.open();
		},
		async logout() {
			inbox.stop();
			// Published while the identity is still known.
			deps.events?.connection("logged-out");
			identity.logout();
			inbox.forgetPosts();
			await deactivateBusTools(deps.pi);
		},
	};
}
