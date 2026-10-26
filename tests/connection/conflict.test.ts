import { describe, expect, test } from "bun:test";
import { createClient } from "../../extension/lib/bus/client.ts";
import { conflictWarning } from "../../extension/lib/connection/conflict.ts";
import { errorResponse, fakeBus, URL_ } from "../fakes.ts";

describe("conflictWarning", () => {
	const active = { url: URL_, token: "tok" };

	test("names the agent from whoami, and omits the name when whoami fails", async () => {
		const named = fakeBus({ tools: { bus_whoami: () => Response.json({ result: { kind: "member", name: "w", spaces: [] } }) } });
		expect(await conflictWarning(createClient(named.fetchFn), active)).toBe("another session is already connected as w; inbox not delivered here");
		const failing = fakeBus({ tools: { bus_whoami: () => errorResponse(500, "internal", "boom") } });
		expect(await conflictWarning(createClient(failing.fetchFn), active)).toBe("another session is already connected as this agent; inbox not delivered here");
	});
});
