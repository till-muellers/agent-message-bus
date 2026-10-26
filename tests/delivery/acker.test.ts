import { describe, expect, test } from "bun:test";
import { createClient } from "../../extension/lib/bus/client.ts";
import { createAcker } from "../../extension/lib/delivery/acker.ts";
import { callsTo, errorResponse, fakeBus, URL_ } from "../fakes.ts";

describe("the acker", () => {
	test("acks the watermark with the session's credentials", async () => {
		const bus = fakeBus();
		const ack = createAcker({ client: createClient(bus.fetchFn), getActive: () => ({ url: URL_, token: "tok" }), warn: () => {} });
		await ack(7);
		expect(callsTo(bus, "/bus_inbox_ack").map(c => c.body)).toEqual([{ upTo: 7 }]);
		expect(callsTo(bus, "/bus_inbox_ack")[0].auth).toBe("Bearer tok");
	});

	test("a failure is only warned; the entries stay on the server", async () => {
		const bus = fakeBus({ tools: { bus_inbox_ack: () => errorResponse(500, "internal", "ack failed") } });
		const warnings: string[] = [];
		const ack = createAcker({ client: createClient(bus.fetchFn), getActive: () => ({ url: URL_, token: "tok" }), warn: m => void warnings.push(m) });
		await ack(7);
		expect(warnings).toEqual(["internal: ack failed"]);
	});

	test("without credentials nothing is acked", async () => {
		const bus = fakeBus();
		const ack = createAcker({ client: createClient(bus.fetchFn), getActive: () => undefined, warn: () => {} });
		await ack(7);
		expect(bus.calls).toEqual([]);
	});
});
