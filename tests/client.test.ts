import { describe, expect, test } from "bun:test";
import { BusError, createClient } from "../extension/lib/client.ts";
import { CODE, errorResponse, fakeBus, URL_ } from "./fakes.ts";

describe("client", () => {
	test("callTool posts JSON with bearer auth and unwraps result; errors become BusError", async () => {
		const bus = fakeBus({
			tools: {
				bus_whoami: () => Response.json({ result: { kind: "admin", spaces: [] } }),
				bus_fail: () => errorResponse(409, "conflict", "exists"),
			},
		});
		const client = createClient(bus.fetchFn);
		expect(await client.callTool(URL_, "t", "bus_whoami", {})).toEqual({ kind: "admin", spaces: [] });
		expect(bus.calls[0]).toMatchObject({ url: `${URL_}/v1/tools/bus_whoami`, method: "POST", auth: "Bearer t" });
		const error = await client.callTool(URL_, "t", "bus_fail", {}).catch(e => e);
		expect(error).toBeInstanceOf(BusError);
		expect(error).toMatchObject({ code: "conflict", message: "exists" });
	});

	test("acceptInvite posts the code as bearer with no body and returns name, space and token", async () => {
		const bus = fakeBus();
		const client = createClient(bus.fetchFn);
		expect(await client.acceptInvite(URL_, "K7M2-X9PQ")).toEqual({ name: "worker", space: "demo", token: "acc-tok" });
		expect(bus.calls).toEqual([{ url: `${URL_}/v1/invites/accept`, method: "POST", auth: "Bearer K7M2-X9PQ", body: undefined }]);
		await expect(client.acceptInvite(URL_, "nope-nope")).rejects.toMatchObject({ code: "unauthenticated", message: "invalid or expired invite code" });
	});

	test("acceptInvite returns the first prompt the invite carries", async () => {
		const client = createClient(fakeBus({ task: "fix the bug" }).fetchFn);
		expect(await client.acceptInvite(URL_, CODE)).toEqual({ name: "worker", space: "demo", token: "acc-tok", task: "fix the bug" });
	});

	test("acceptInvite rejects a response without name, space and token, or with a task that is not a string", async () => {
		for (const [body, message] of [
			[{ name: "worker", space: "demo" }, "accept: response has no token"],
			[{ name: "worker", space: "demo", token: "acc-tok", task: 7 }, "accept: task must be a string"],
		] as const) {
			const error = await createClient(async () => Response.json(body))
				.acceptInvite(URL_, CODE)
				.catch(e => e);
			expect(error).toBeInstanceOf(BusError);
			expect(error).toMatchObject({ code: "invalid_response", message });
		}
	});
});
