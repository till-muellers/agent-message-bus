import { describe, expect, test } from "bun:test";
import type { BusError } from "../../extension/lib/bus/client.ts";
import { createClient } from "../../extension/lib/bus/client.ts";
import { runInboxLoop } from "../../extension/lib/delivery/inbox-loop.ts";
import type { Entry } from "../../extension/lib/bus/entry.ts";
import { callsTo, entry, errorResponse, fakeBus, frame, streamOf, type ToolHandler, URL_ } from "../fakes.ts";

const conflict = () => errorResponse(409, "conflict", "agent w already has an active inbox stream");

/** Runs the loop against a fake bus until its scripted streams run out; records deliveries, errors, conflicts and waits. */
async function runLoop(streams: (() => Response)[], tools: Record<string, ToolHandler> = {}, signal = new AbortController().signal) {
	const bus = fakeBus({ streams, tools });
	const delivered: number[] = [];
	const errors: string[] = [];
	const conflicts: string[] = [];
	const delays: number[] = [];
	await runInboxLoop({
		client: createClient(bus.fetchFn),
		getActive: () => ({ url: URL_, token: "tok" }),
		signal,
		deliver: async (e: Entry) => void delivered.push(e.id),
		onError: error => void errors.push(error.message),
		onConflict: async (error: BusError) => void conflicts.push(error.code),
		sleep: async ms => void delays.push(ms),
	});
	return { bus, delivered, errors, conflicts, delays };
}

describe("runInboxLoop", () => {
	test("delivers in order, acks each id, reconnects, and stops on 401", async () => {
		const { bus, delivered, errors } = await runLoop([
			() => new Response(streamOf([": ping\n\n", frame(entry(1)), frame(entry(2, { critical: true }))])),
			() => new Response(streamOf([frame(entry(3))])),
		]);
		expect(delivered).toEqual([1, 2, 3]);
		const acks = callsTo(bus, "/bus_inbox_ack");
		expect(acks.map(c => c.body)).toEqual([{ upTo: 1 }, { upTo: 2 }, { upTo: 3 }]);
		expect(acks.every(c => c.auth === "Bearer tok")).toBe(true);
		expect(errors).toEqual(["invalid token"]);
	});

	test("retries other errors with doubling backoff capped at 30s", async () => {
		const boom = () => errorResponse(500, "internal", "boom");
		const { delays, errors } = await runLoop([boom, boom, boom, boom, boom, boom, () => errorResponse(403, "forbidden", "admin")]);
		expect(delays).toEqual([1000, 2000, 4000, 8000, 16000, 30000]);
		expect(errors.at(-1)).toBe("admin");
	});

	test("returns immediately when aborted", async () => {
		const controller = new AbortController();
		controller.abort();
		const { bus } = await runLoop([], {}, controller.signal);
		expect(bus.calls).toHaveLength(0);
	});

	test("an ack failure is reported without reconnecting or redelivering", async () => {
		const { bus, delivered, errors } = await runLoop([() => new Response(streamOf([frame(entry(1)), frame(entry(2))]))], {
			bus_inbox_ack: args => (args.upTo === 1 ? errorResponse(500, "internal", "ack failed") : Response.json({ result: { acked: 2 } })),
		});
		expect(delivered).toEqual([1, 2]);
		expect(callsTo(bus, "/bus_inbox_ack").map(c => c.body)).toEqual([{ upTo: 1 }, { upTo: 2 }]);
		expect(errors).toEqual(["ack failed", "invalid token"]);
	});

	test("a stream that opens without frames still ends the conflict streak", async () => {
		const { conflicts } = await runLoop([conflict, () => new Response(streamOf([": ping\n\n"])), conflict]);
		expect(conflicts).toEqual(["conflict", "conflict"]);
	});

	test("a 409 conflict is reported once per streak, retried with backoff, and a successful open resets the streak", async () => {
		const { delivered, errors, conflicts, delays } = await runLoop([conflict, conflict, () => new Response(streamOf([frame(entry(1))])), conflict]);
		expect(delivered).toEqual([1]);
		expect(conflicts).toEqual(["conflict", "conflict"]);
		expect(delays).toEqual([1000, 2000, 1000, 2000]);
		expect(errors).toEqual(["invalid token"]);
	});

	test("401 and 403 stay fatal", async () => {
		for (const [status, code] of [
			[401, "unauthenticated"],
			[403, "forbidden"],
		] as const) {
			const { bus, delivered } = await runLoop([() => errorResponse(status, code, "no"), () => new Response(streamOf([frame(entry(1))]))]);
			expect({ code, delivered, opens: bus.calls.length }).toEqual({ code, delivered: [], opens: 1 });
		}
	});
});

