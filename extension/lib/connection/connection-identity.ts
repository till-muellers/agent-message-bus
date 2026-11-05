/**
 * Who the connection acts as. A main session resolves its own credentials and publishes them for its subagents
 * (`root-publication.ts`); a subagent reads its root session's identity at every call, never the store or the env,
 * so a later promote or logout of the main session reaches its next call.
 */
import type { ActiveCredentials } from "../bus/credentials.ts";
import type { RootIdentities } from "../session/root-identities.ts";
import { rootKeyOf } from "../session/root-identities.ts";
import type { SessionCredentials } from "../session/session-credentials.ts";
import type { SessionIdentity } from "../session/session-identity.ts";
import type { CredentialStore } from "../session/stored-credentials.ts";
import { createRootPublication } from "./root-publication.ts";
import type { SessionView } from "./session-view.ts";

export interface ConnectionIdentityDeps {
	store: CredentialStore;
	credentials: Pick<SessionCredentials, "resolve">;
	identity: Pick<SessionIdentity, "load">;
	/** The identities root sessions publish for their subagents; shared by every session of the process. */
	roots: RootIdentities;
	view: SessionView;
	warn(message: string): void;
}

export interface ConnectionIdentity {
	/** The credentials active at call time; undefined while the session has none. */
	getActive(): ActiveCredentials | undefined;
	/** Resolves the credentials of a new connection start; a subagent without its root's identity is told once. */
	resolve(): Promise<ActiveCredentials | undefined>;
	/** Withdraws the identity published for the subagents. */
	withdraw(): void;
	/** Drops the credentials and withdraws what was published. */
	logout(): void;
}

export function createConnectionIdentity(deps: ConnectionIdentityDeps): ConnectionIdentity {
	const { view } = deps;
	const publication = createRootPublication(deps.roots);
	let active: ActiveCredentials | undefined;
	let toldNoIdentity = false;
	const getActive = () => (view.sub ? deps.roots.lookup(view.sessionFile) : active);
	/** A subagent without its root session's identity gets no bus tools: one line, then silence. */
	const noIdentity = (): undefined => {
		if (toldNoIdentity) return undefined;
		toldNoIdentity = true;
		deps.warn("no bus tools for the subagent: its main session has no bus identity");
		return undefined;
	};
	return {
		getActive,
		async resolve() {
			if (view.sub) return getActive() ?? noIdentity();
			await deps.identity.load();
			active = deps.credentials.resolve(await deps.store.load());
			publication.set(rootKeyOf(view.sessionFile), active);
			return active;
		},
		withdraw: publication.clear,
		logout() {
			active = undefined;
			publication.clear();
		},
	};
}
