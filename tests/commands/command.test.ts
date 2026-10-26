import { describe, expect, test } from "bun:test";
import { type BusClient, createClient } from "../../extension/lib/bus/client.ts";
import { type CommandDeps, COMMAND_DESCRIPTION, runCommand, USAGE } from "../../extension/lib/commands/command.ts";
import { createSessionCredentials, type SessionCredentials } from "../../extension/lib/session/session-credentials.ts";
import type { CredentialStore } from "../../extension/lib/session/stored-credentials.ts";
import type { ServerCli } from "../../extension/lib/local-server/server-cli.ts";
import { CODE, errorResponse, fakeBus, memoryStore, URL_ } from "../fakes.ts";

function commandDeps(store: CredentialStore, client: BusClient, session: SessionCredentials = createSessionCredentials(undefined), server: ServerCli = async () => "") {
	const deps = {
		store,
		client,
		session,
		server,
		restarts: 0,
		stops: 0,
		acceptedCleared: 0,
		accepts: [] as string[][],
		spawns: [] as unknown[],
		pluginVersion: "0.2.0",
		installedPluginVersion: async () => undefined as string | undefined,
		gate: { check: async () => "ok" as const, last: () => ({ url: URL_, protocol: 2 as number | undefined }) },
		restart: async () => void deps.restarts++,
		stop: async () => void deps.stops++,
		accept: async (url: string, code: string) => {
			deps.accepts.push([url, code]);
			return { message: "accepted", level: "info" as const };
		},
		clearAccepted: async () => {
			deps.session.setAccepted(undefined);
			deps.acceptedCleared++;
		},
		spawn: async (req: { space: string; name: string; task?: string }) => {
			deps.spawns.push(req);
			return { name: req.name, space: req.space, launcher: "tmux" };
		},
	} satisfies CommandDeps & Record<string, unknown>;
	return deps;
}

const whoami = (byToken: Record<string, unknown>) => (_args: Record<string, unknown>, auth: string | undefined) =>
	Response.json({ result: byToken[auth?.replace("Bearer ", "") ?? ""] });
const clientWith = (tools: Parameters<typeof fakeBus>[0] = {}) => createClient(fakeBus(tools).fetchFn);

describe("runCommand", () => {
	test("login with an admin token stores adminToken and restarts, so the tools follow the resolved credentials", async () => {
		const store = memoryStore();
		const deps = commandDeps(store, clientWith({ tools: { bus_whoami: whoami({ adm: { kind: "admin", spaces: [] } }) } }));
		const result = await runCommand(`login ${URL_}/ adm`, deps);
		expect(result).toEqual({ message: "Admin token stored. Run /bus promote <leader-name> to act as a leader.", level: "info" });
		expect(store.value).toEqual({ url: URL_, adminToken: "adm", token: undefined });
		expect(deps.restarts).toBe(1);
	});

	test("login with a member token stores token and restarts; the admin token is kept only for the same url", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", space: "demo", spaces: [{ name: "demo", role: "member" }] } }) } });
		for (const [storedUrl, adminToken] of [
			["http://old", undefined],
			[URL_, "adm"],
		] as const) {
			const store = memoryStore({ url: storedUrl, adminToken: "adm" });
			const deps = commandDeps(store, client);
			const result = await runCommand(["login", URL_, "mem"], deps);
			expect(result.message).toBe("Logged in as member w");
			expect({ storedUrl, value: store.value }).toEqual({ storedUrl, value: { url: URL_, adminToken, token: "mem" } });
			expect(deps.restarts).toBe(1);
		}
	});

	test("promote without an admin token is an error", async () => {
		const deps = commandDeps(memoryStore({ url: URL_, token: "x" }), clientWith());
		expect(await runCommand("promote lead", deps)).toEqual({ message: "Run /bus login <url> <admin-token> first.", level: "error" });
	});

	test("promote mints a leader with the admin token into the session only; the store is untouched", async () => {
		const bus = fakeBus({ tools: { bus_mint_leader: args => Response.json({ result: { name: args.name, token: "lead-tok" } }) } });
		const store = memoryStore({ url: `${URL_}/`, adminToken: "adm" });
		const deps = commandDeps(store, createClient(bus.fetchFn));
		const result = await runCommand("promote lead", deps);
		expect(result).toEqual({
			message: "This session now acts as leader lead (until it ends; run /bus promote again after a restart).",
			level: "info",
		});
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_mint_leader`, auth: "Bearer adm", body: { name: "lead" } });
		expect(store.value).toEqual({ url: `${URL_}/`, adminToken: "adm" });
		expect(deps.session.resolve(undefined)).toEqual({ url: URL_, token: "lead-tok", source: "promote" });
		expect(deps.restarts).toBe(1);
	});

	test("status shows the session's promoted identity over the stored one", async () => {
		const client = clientWith({
			tools: { bus_whoami: whoami({ "lead-tok": { kind: "leader", name: "lead", spaces: [] }, mem: { kind: "member", name: "w", spaces: [] } }) },
		});
		const promoted = createSessionCredentials(undefined);
		promoted.promote({ url: URL_, token: "lead-tok" });
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), client, promoted);
		expect((await runCommand("status", deps)).message).toBe(
			`${URL_}: leader lead; spaces: none\nserver protocol 2 (minimum 2)\nextension 0.2.0\ninstalled plugin unknown`,
		);
	});

	test("status shows the extension version, the installed plugin version and the warning when the installed plugin is older", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) } });
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), client);
		deps.installedPluginVersion = async () => "0.1.0";
		expect((await runCommand("status", deps)).message).toBe(
			`${URL_}: member w; spaces: none\nserver protocol 2 (minimum 2)\nextension 0.2.0\ninstalled plugin 0.1.0\nthe installed omp plugin 0.1.0 is older than the extension protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus`,
		);
	});

	test("status shows the server protocol and the minimum it needs", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) } });
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), client);
		deps.gate = { check: async () => "ok", last: () => ({ url: URL_, protocol: 2 }) };
		expect((await runCommand("status", deps)).message).toContain("server protocol 2 (minimum 2)");
	});

	test("status shows a server without a protocol as none", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) } });
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), client);
		deps.gate = { check: async () => "ok", last: () => ({ url: URL_, protocol: undefined as number | undefined }) };
		expect((await runCommand("status", deps)).message).toContain("server protocol none (minimum 2)");
	});

	test("status shows no warning when the installed plugin matches the extension", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) } });
		const deps = commandDeps(memoryStore({ url: URL_, token: "mem" }), client);
		deps.installedPluginVersion = async () => "0.2.0";
		expect((await runCommand("status", deps)).message).toBe(
			`${URL_}: member w; spaces: none\nserver protocol 2 (minimum 2)\nextension 0.2.0\ninstalled plugin 0.2.0`,
		);
	});

	test("logout clears the stored file, the session override and the env credentials with no fallback, and stops", async () => {
		const store = memoryStore({ url: URL_, adminToken: "adm", token: "t" });
		const session = createSessionCredentials({ url: URL_, token: "env" });
		session.promote({ url: URL_, token: "lead-tok" });
		const deps = commandDeps(store, clientWith(), session);
		expect(await runCommand("logout", deps)).toEqual({ message: "Logged out; bus tools deactivated.", level: "info" });
		expect({ session: deps.session.resolve(undefined), stored: store.value }).toEqual({ session: undefined, stored: undefined });
		expect({ restarts: deps.restarts, stops: deps.stops }).toEqual({ restarts: 0, stops: 1 });
	});

	test("login with an agent token clears the session override so the new token takes effect", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] } }) } });
		const promoted = createSessionCredentials(undefined);
		promoted.promote({ url: URL_, token: "lead-tok" });
		const deps = commandDeps(memoryStore(), client, promoted);
		await runCommand(`login ${URL_} mem`, deps);
		expect(deps.session.resolve(undefined)).toBeUndefined();
		expect(deps.restarts).toBe(1);
	});

	test("login with an agent token and logout clear the accepted token; login with an admin token keeps it", async () => {
		const client = clientWith({ tools: { bus_whoami: whoami({ mem: { kind: "member", name: "w", spaces: [] }, adm: { kind: "admin", spaces: [] } }) } });
		const accepted = { url: URL_, token: "acc-tok" };
		const session = createSessionCredentials(undefined);
		session.setAccepted(accepted);
		const deps = commandDeps(memoryStore(), client, session);
		await runCommand(`login ${URL_} adm`, deps);
		expect({ accepted: deps.session.resolve(undefined), cleared: deps.acceptedCleared }).toEqual({ accepted: { ...accepted, source: "accepted" }, cleared: 0 });
		await runCommand(`login ${URL_} mem`, deps);
		expect({ accepted: deps.session.resolve(undefined), cleared: deps.acceptedCleared }).toEqual({ accepted: undefined, cleared: 1 });
		await runCommand("logout", deps);
		expect(deps.acceptedCleared).toBe(2);
	});

	test("accept passes url and code to deps.accept; a missing argument shows usage as an error", async () => {
		const deps = commandDeps(memoryStore(), clientWith());
		expect(await runCommand(`accept ${URL_} ${CODE}`, deps)).toEqual({ message: "accepted", level: "info" });
		expect(deps.accepts).toEqual([[URL_, CODE]]);
		expect(await runCommand(`accept ${URL_}`, deps)).toEqual({ message: USAGE, level: "error" });
		expect(await runCommand("accept", deps)).toEqual({ message: USAGE, level: "error" });
		expect(deps.accepts).toHaveLength(1);
	});

	test("accept errors from the server surface as <code>: <message>", async () => {
		const client = clientWith();
		const deps = { ...commandDeps(memoryStore(), client), accept: () => client.acceptInvite(URL_, "nope-nope").then(() => ({ message: "", level: "info" as const })) };
		expect(await runCommand(`accept ${URL_} nope-nope`, deps)).toEqual({ message: "unauthenticated: invalid or expired invite code", level: "error" });
	});

	test("spawn passes space, name and the rest as the task; reports the launcher; missing arguments show usage", async () => {
		const deps = commandDeps(memoryStore(), clientWith());
		expect(await runCommand("spawn demo worker fix the bug", deps)).toEqual({ message: "Started worker worker in space demo (tmux).", level: "info" });
		expect(await runCommand("spawn demo w2", deps)).toEqual({ message: "Started worker w2 in space demo (tmux).", level: "info" });
		expect(await runCommand("spawn demo", deps)).toEqual({ message: USAGE, level: "error" });
		expect(deps.spawns).toEqual([
			{ space: "demo", name: "worker", task: "fix the bug" },
			{ space: "demo", name: "w2", task: undefined },
		]);
	});

	test("spawn errors surface as error messages", async () => {
		const deps = {
			...commandDeps(memoryStore(), clientWith()),
			spawn: async () => {
				throw new Error("no terminal launcher found for this environment");
			},
		};
		expect(await runCommand("spawn demo worker", deps)).toEqual({ message: "no terminal launcher found for this environment", level: "error" });
	});

	test("status shows whoami; no credentials shows usage; logout clears; unknown is an error", async () => {
		const client = clientWith({ tools: { bus_whoami: () => Response.json({ result: { kind: "leader", name: "lead", spaces: [{ name: "demo", role: "leader" }] } }) } });
		const store = memoryStore({ url: URL_, token: "t" });
		const deps = commandDeps(store, client);
		const status = await runCommand("", deps);
		expect(status.level).toBe("info");
		expect(status.message).toContain("leader lead");
		expect(status.message).toContain("demo (leader)");
		expect(await runCommand("logout", deps)).toEqual({ message: "Logged out; bus tools deactivated.", level: "info" });
		expect(store.value).toBeUndefined();
		expect(deps.stops).toBe(1);
		expect((await runCommand("status", deps)).message).toContain("/bus login");
		expect((await runCommand("frobnicate", deps)).level).toBe("error");
	});

	test("subcommands are case-insensitive", async () => {
		const deps = commandDeps(memoryStore(), clientWith());
		expect(await runCommand("HELP", deps)).toEqual({ message: USAGE, level: "info" });
	});

	test("usage and the command description list every subcommand, from the same table", async () => {
		const deps = commandDeps(memoryStore(), clientWith());
		expect(await runCommand("help", deps)).toEqual({ message: USAGE, level: "info" });
		const names = ["login", "accept", "spawn", "promote", "server", "status", "logout", "help"];
		expect(USAGE.split("\n").slice(1).map(line => line.trim().split(" ")[1])).toEqual(names);
		expect(COMMAND_DESCRIPTION.slice("Agent message bus: ".length).split(" | ").map(s => s.split(" ")[0])).toEqual(names);
	});

	test("server errors surface as error messages", async () => {
		const deps = commandDeps(memoryStore(), clientWith({ tools: { bus_whoami: () => errorResponse(401, "unauthenticated", "invalid token") } }));
		expect(await runCommand(`login ${URL_} bad`, deps)).toEqual({ message: "unauthenticated: invalid token", level: "error" });
	});
});
