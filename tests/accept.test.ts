import { describe, expect, test } from "bun:test";
import { acceptInvite } from "../extension/lib/commands/accept.ts";
import { createClient } from "../extension/lib/bus/client.ts";
import type { ActiveCredentials, SessionCredentials } from "../extension/lib/session/stored-credentials.ts";
import { CODE, fakeBus, URL_ } from "./fakes.ts";

function setup(options: { task?: string; persisted?: boolean } = {}) {
	const bus = fakeBus({ task: options.task });
	const saved: ActiveCredentials[] = [];
	const session: SessionCredentials = { override: { url: URL_, token: "lead-tok" }, env: { url: URL_, token: "env-tok" } };
	const identity = {
		save: async (creds: ActiveCredentials) => {
			saved.push(creds);
			return options.persisted ?? true;
		},
	};
	return { bus, saved, session, deps: { client: createClient(bus.fetchFn), session, identity } };
}

describe("acceptInvite", () => {
	test("redeems the code at the trimmed url, saves the member token and drops the promoted and env credentials", async () => {
		const { bus, saved, session, deps } = setup();
		expect(await acceptInvite(deps, `${URL_}/`, CODE)).toEqual({ result: { message: "Joined space demo as member worker.", level: "info" } });
		expect(bus.calls.map(c => c.url)).toEqual([`${URL_}/v1/invites/accept`]);
		expect(saved).toEqual([{ url: URL_, token: "acc-tok" }]);
		expect(session).toEqual({ override: undefined, env: undefined });
	});

	test("returns the first prompt that travelled with the invite", async () => {
		const { deps } = setup({ task: "fix the bug" });
		expect((await acceptInvite(deps, URL_, CODE)).task).toBe("fix the bug");
	});

	test("without a session file the reply says the token is not persisted", async () => {
		const { deps } = setup({ persisted: false });
		expect((await acceptInvite(deps, URL_, CODE)).result.message).toBe("Joined space demo as member worker. Not persisted: this session has no session file.");
	});

	test("a rejected code saves nothing and keeps the session's credentials", async () => {
		const { saved, session, deps } = setup();
		await expect(acceptInvite(deps, URL_, "nope-nope")).rejects.toMatchObject({ code: "unauthenticated" });
		expect(saved).toEqual([]);
		expect(session.override).toEqual({ url: URL_, token: "lead-tok" });
	});
});
