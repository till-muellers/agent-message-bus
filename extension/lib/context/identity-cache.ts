/**
 * The bus identity the context block is built from (`docs/design/arch/client.md` §5): read with `bus_whoami`
 * (plus the leaders of a member's space, or every space for an admin) when the connection restarts or a
 * membership tool succeeded, and kept between, so the block never waits for the network. A failed refresh
 * keeps the last identity; a revoked token drops it.
 */
import { BusError, type BusClient } from "../bus/client.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";

type Role = "admin" | "leader" | "member";

export interface BusIdentity {
	kind: Role;
	name: string;
	url: string;
	spaces: { name: string; role: Role }[];
	/** A member's space and its leaders. */
	space?: string;
	leadersOfSpace?: string[];
	/** An admin's view: every space of the bus. */
	allSpaces?: string[];
}

export interface IdentityCache {
	/** The identity read with the active credentials; undefined without credentials or before a refresh. Never blocks. */
	current(): BusIdentity | undefined;
	/** Reads the identity again; failures keep the last one, except a revoked token. */
	refresh(): Promise<void>;
	clear(): void;
}

export interface IdentityCacheDeps {
	client: BusClient;
	getActive(): ActiveCredentials | undefined;
}

interface Who {
	kind: Role;
	name: string;
	space?: string;
	spaces: { name: string; role: Role }[];
}

function isWho(value: unknown): value is Who {
	const who = value as Partial<Who> | null;
	return typeof who?.name === "string" && (who.kind === "admin" || who.kind === "leader" || who.kind === "member") && Array.isArray(who.spaces);
}

const names = (value: unknown, keep: (item: { name?: unknown; role?: unknown }) => boolean = () => true): string[] =>
	Array.isArray(value) ? value.filter(keep).flatMap(item => (typeof item?.name === "string" ? [item.name] : [])) : [];

/** The identity behind `active`, with the extra reads its kind needs. */
async function readIdentity(client: BusClient, active: ActiveCredentials): Promise<BusIdentity> {
	const call = (name: string, args: Record<string, unknown> = {}) => client.callTool(active.url, active.token, name, args);
	const who = await call("bus_whoami");
	if (!isWho(who)) throw new BusError("invalid_response", "bus_whoami: unexpected result");
	const identity: BusIdentity = { kind: who.kind, name: who.name, url: active.url, spaces: who.spaces };
	if (who.kind === "admin") return { ...identity, allSpaces: names(await call("bus_list_spaces")) };
	const space = who.space ?? who.spaces[0]?.name;
	if (who.kind !== "member" || space === undefined) return identity;
	const leaders = names(await call("bus_list_participants", { space }), item => item.role === "leader");
	return { ...identity, space, leadersOfSpace: leaders };
}

export function createIdentityCache(deps: IdentityCacheDeps): IdentityCache {
	let cached: { token: string; identity: BusIdentity } | undefined;
	// A refresh that a clear or a later refresh overtook must not write its result.
	let generation = 0;
	return {
		current() {
			const active = deps.getActive();
			return active && cached?.token === active.token ? cached.identity : undefined;
		},
		async refresh() {
			const mine = ++generation;
			const active = deps.getActive();
			if (!active) {
				cached = undefined;
				return;
			}
			try {
				const identity = await readIdentity(deps.client, active);
				if (mine === generation) cached = { token: active.token, identity };
			} catch (error) {
				if (mine === generation && error instanceof BusError && error.code === "unauthenticated") cached = undefined;
			}
		},
		clear() {
			generation++;
			cached = undefined;
		},
	};
}
