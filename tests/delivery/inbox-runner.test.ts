import { describe, expect, test } from "bun:test";
import { createClient } from "../../extension/lib/bus/client.ts";
import { createInboxRunner, type StartSink } from "../../extension/lib/delivery/inbox-runner.ts";
import { fakeBus, liveStream, noSleep, URL_ } from "../fakes.ts";

const sink: StartSink = { deliver: () => {}, stop: () => {} };

describe("the inbox runner", () => {
	test("the stream counts as open while it is read; the end of a stopped loop's stream does not close the next one", async () => {
		const first = liveStream();
		const second = liveStream();
		const bus = fakeBus({ streams: [first.response, second.response] });
		const runner = createInboxRunner({ client: createClient(bus.fetchFn), getActive: () => ({ url: URL_, token: "t" }), warn: () => {}, sleep: noSleep });
		expect(runner.isOpen()).toBe(false);
		runner.start(sink);
		await first.opened;
		expect(runner.isOpen()).toBe(true);
		runner.stop();
		expect(runner.isOpen()).toBe(false);
		runner.start(sink);
		await second.opened;
		first.close();
		// The stopped loop reports its stream's end on its own schedule and exposes no signal for it.
		await Bun.sleep(10);
		expect(runner.isOpen()).toBe(true);
		runner.stop();
		second.close();
	});

	test("stop stops the sink of the start, once", () => {
		let stopped = 0;
		const holding: StartSink = { ...sink, stop: () => void stopped++ };
		const runner = createInboxRunner({ client: createClient(fakeBus().fetchFn), getActive: () => undefined, warn: () => {}, sleep: noSleep });
		runner.start(holding);
		runner.stop();
		runner.stop();
		expect(stopped).toBe(1);
	});
});
