/** The cached bus identity the context block is built from (`client.md` §5). */
import { describe, expect, test } from "bun:test";
import { createClient, type FetchLike } from "../../extension/lib/bus/client.ts";
import { createIdentityCache } from "../../extension/lib/context/identity-cache.ts";
import type { ActiveCredentials } from "../../extension/lib/session/stored-credentials.ts";
import { callsTo, errorResponse, fakeBus, URL_ } from "../fakes.ts";

type Tools = NonNullable<Parameters<typeof fakeBus>[0]>["tools"];

const whoami = (result: unknown) => () => Response.json({ result });
const memberWho = { kind: "member", name: "worker", space: "demo", spaces: [{ name: "demo", role: "member" }] };
const participants = () =>
	Response.json({
		result: [
			{ name: "ops", role: "admin" },
			{ name: "lead", role: "leader" },
			{ name: "worker", role: "member" },
		],
	});

function cache(tools: Tools, active: { value: ActiveCredentials | undefined } = { value: { url: URL_, token: "t" } }) {
	const bus = fakeBus({ tools });
	const it = createIdentityCache({ client: createClient(bus.fetchFn), getActive: () => active.value });
	return { it, bus, active };
}

describe("the identity cache", () => {
	test("a member's identity names its space and that space's leaders", async () => {
		const { it, bus } = cache({ bus_whoami: whoami(memberWho), bus_list_participants: participants });
		expect(it.current()).toBeUndefined();
		await it.refresh();
		expect(it.current()).toEqual({ kind: "member", name: "worker", url: URL_, spaces: [{ name: "demo", role: "member" }], space: "demo", leadersOfSpace: ["lead"] });
		expect(callsTo(bus, "/bus_list_participants").map(c => c.body)).toEqual([{ space: "demo" }]);
	});

	test("an admin's identity lists every space; a leader's needs nothing but whoami", async () => {
		const admin = cache({
			bus_whoami: whoami({ kind: "admin", name: "ops", spaces: [] }),
			bus_list_spaces: () => Response.json({ result: [{ name: "demo" }, { name: "beta" }] }),
		});
		await admin.it.refresh();
		expect(admin.it.current()).toEqual({ kind: "admin", name: "ops", url: URL_, spaces: [], allSpaces: ["demo", "beta"] });
		const leader = cache({ bus_whoami: whoami({ kind: "leader", name: "lead", spaces: [{ name: "demo", role: "leader" }] }) });
		await leader.it.refresh();
		expect(leader.it.current()).toEqual({ kind: "leader", name: "lead", url: URL_, spaces: [{ name: "demo", role: "leader" }] });
		expect(leader.bus.calls.map(c => c.url.slice(c.url.lastIndexOf("/") + 1))).toEqual(["bus_whoami"]);
	});

	test("a failed refresh keeps the last identity", async () => {
		let fail = false;
		const failing: FetchLike = async () => {
			throw new TypeError("network down");
		};
		const bus = fakeBus({ tools: { bus_whoami: whoami(memberWho), bus_list_participants: participants } });
		const it = createIdentityCache({ client: createClient((url, init) => (fail ? failing(url, init) : bus.fetchFn(url, init))), getActive: () => ({ url: URL_, token: "t" }) });
		await it.refresh();
		fail = true;
		await it.refresh();
		expect(it.current()?.name).toBe("worker");
	});

	test("a revoked token drops the identity", async () => {
		let revoked = false;
		const { it } = cache({
			bus_whoami: () => (revoked ? errorResponse(401, "unauthenticated", "invalid token") : Response.json({ result: memberWho })),
			bus_list_participants: participants,
		});
		await it.refresh();
		revoked = true;
		await it.refresh();
		expect(it.current()).toBeUndefined();
	});

	test("the identity belongs to the credentials it was read with: other or no credentials show none", async () => {
		const { it, active } = cache({ bus_whoami: whoami(memberWho), bus_list_participants: participants });
		await it.refresh();
		active.value = { url: URL_, token: "other" };
		expect(it.current()).toBeUndefined();
		active.value = undefined;
		expect(it.current()).toBeUndefined();
	});

	test("clear drops the identity, and a refresh that was running when it cleared does not bring it back", async () => {
		const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
		const { it } = cache({ bus_whoami: () => answered, bus_list_participants: participants });
		const running = it.refresh();
		it.clear();
		answer(Response.json({ result: memberWho }));
		await running;
		expect(it.current()).toBeUndefined();
	});
});
