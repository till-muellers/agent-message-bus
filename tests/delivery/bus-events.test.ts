/** Bus activity on omp's extension event bus (`client.md` §3.6, `omp-parity/omp-events.feature`). */
import { describe, expect, test } from "bun:test";
import { announceInstance, createBusEvents } from "../../extension/lib/delivery/bus-events.ts";
import { entry, fakeEvents } from "../fakes.ts";

describe("the bus events", () => {
	test("a handed-over entry is published with its delivery mode", () => {
		const events = fakeEvents();
		const seen: unknown[] = [];
		events.on("agent-bus:entry", data => void seen.push(data));
		createBusEvents(events, () => ({}), () => {}).entry(entry(1), "aside");
		expect(seen).toEqual([{ entry: entry(1), deliverAs: "aside" }]);
	});

	test("a connection state is published with the agent name and space", () => {
		const events = fakeEvents();
		const seen: unknown[] = [];
		events.on("agent-bus:connection", data => void seen.push(data));
		const bus = createBusEvents(events, () => ({ agent: "worker", space: "demo" }), () => {});
		bus.connection("connected");
		bus.connection("logged-out");
		expect(seen).toEqual([
			{ state: "connected", agent: "worker", space: "demo" },
			{ state: "logged-out", agent: "worker", space: "demo" },
		]);
	});

	test("a listener that throws is logged and does not reach the publisher", () => {
		const events = fakeEvents();
		const warnings: string[] = [];
		events.on("agent-bus:entry", () => {
			throw new Error("listener broke");
		});
		expect(() => createBusEvents(events, () => ({}), m => void warnings.push(m)).entry(entry(1), "steer")).not.toThrow();
		expect(warnings).toEqual(["agent-bus:entry listener failed: listener broke"]);
	});
});

describe("the instance handshake", () => {
	test("the first copy on an event bus stays active; every later copy defers to it", () => {
		const events = fakeEvents();
		const first = announceInstance(events);
		const second = announceInstance(events);
		const third = announceInstance(events);
		expect([first.deferred(), second.deferred(), third.deferred()]).toEqual([false, true, true]);
	});

	test("copies on different event buses do not see each other", () => {
		expect([announceInstance(fakeEvents()).deferred(), announceInstance(fakeEvents()).deferred()]).toEqual([false, false]);
	});
});
