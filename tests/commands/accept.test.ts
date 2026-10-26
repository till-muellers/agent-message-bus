import { describe, expect, test } from "bun:test";
import { acceptInvite } from "../../extension/lib/commands/accept.ts";
import { createClient } from "../../extension/lib/bus/client.ts";
import { createSessionCredentials } from "../../extension/lib/session/session-credentials.ts";
import type { ActiveCredentials } from "../../extension/lib/session/stored-credentials.ts";
import { CODE, fakeBus, URL_ } from "../fakes.ts";
import type { ServerGate } from "../../extension/lib/connection/server-gate.ts";

/** A gate that refuses the server the way a too old one does. */
const refusingGate: ServerGate = { check: async () => ({ refused: "too old" }), last: () => undefined };

function setup(options: { task?: string; persisted?: boolean } = {}) {
	const bus = fakeBus({ task: options.task });
	const saved: ActiveCredentials[] = [];
	const session = createSessionCredentials({ url: URL_, token: "env-tok" });
	session.promote({ url: URL_, token: "lead-tok" });
	const identity = {
		save: async (creds: ActiveCredentials) => {
			saved.push(creds);
			return options.persisted ?? true;
		},
	};
	const gate: ServerGate = { check: async () => "ok", last: () => undefined };
	return { bus, saved, session, gate, deps: { client: createClient(bus.fetchFn), session, identity, gate } };
}

describe("acceptInvite", () => {
	test("redeems the code at the trimmed url, saves the member token and drops the promoted and env credentials", async () => {
		const { bus, saved, session, deps } = setup();
		expect(await acceptInvite(deps, `${URL_}/`, CODE)).toEqual({ result: { message: "Joined space demo as member worker.", level: "info" } });
		expect(bus.calls.map(c => c.url)).toEqual([`${URL_}/v1/invites/accept`]);
		expect(saved).toEqual([{ url: URL_, token: "acc-tok" }]);
		expect(session.resolve(undefined)).toBeUndefined();
	});

	test("returns the first prompt that travelled with the invite", async () => {
		const { deps } = setup({ task: "fix the bug" });
		expect((await acceptInvite(deps, URL_, CODE)).task).toBe("fix the bug");
	});

	test("without a session file the reply says the token is not persisted", async () => {
		const { deps } = setup({ persisted: false });
		expect((await acceptInvite(deps, URL_, CODE)).result.message).toBe("Joined space demo as member worker. Not persisted: this session has no session file.");
	});

	test("a server that is too old is refused before the code is posted", async () => {
		const { bus, saved, deps } = setup();
		deps.gate = refusingGate;
		await expect(acceptInvite(deps, URL_, CODE)).rejects.toThrow("too old");
		expect(bus.calls).toEqual([]);
		expect(saved).toEqual([]);
	});

	test("a rejected code saves nothing and keeps the session's credentials", async () => {
		const { saved, session, deps } = setup();
		await expect(acceptInvite(deps, URL_, "nope-nope")).rejects.toMatchObject({ code: "unauthenticated" });
		expect(saved).toEqual([]);
		expect(session.resolve(undefined)).toEqual({ url: URL_, token: "lead-tok", source: "promote" });
	});
});
