import { describe, expect, test } from "bun:test";
import { createAckWatermark } from "../../extension/lib/delivery/ack-watermark.ts";

describe("the ack watermark", () => {
	test("acks the entries in order", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(2);
		expect(watermark.consumed([1])).toBe(1);
		expect(watermark.consumed([2])).toBe(2);
	});

	test("nothing is acked before an entry arrived", () => {
		const watermark = createAckWatermark();
		expect(watermark.consumed([1])).toBeUndefined();
	});

	test("an interrupt consumed before the older entries acks nothing", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(2);
		expect(watermark.consumed([2])).toBeUndefined();
		expect(watermark.consumed([1])).toBe(2);
	});

	test("a hole holds the watermark until it is filled", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(2);
		watermark.received(3);
		expect(watermark.consumed([1, 3])).toBe(1);
		expect(watermark.consumed([3])).toBeUndefined();
		expect(watermark.consumed([2])).toBe(3);
	});

	test("duplicates and unknown ids change nothing", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(2);
		expect(watermark.consumed([1, 1])).toBe(1);
		expect(watermark.consumed([1])).toBeUndefined();
		expect(watermark.consumed([99])).toBeUndefined();
		expect(watermark.consumed([2])).toBe(2);
	});

	test("an entry that never arrived does not hold the watermark", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(3);
		expect(watermark.consumed([1, 3])).toBe(3);
	});

	test("a replayed entry does not move the watermark twice", () => {
		const watermark = createAckWatermark();
		watermark.received(1);
		watermark.received(2);
		expect(watermark.consumed([1])).toBe(1);
		watermark.received(1);
		watermark.received(2);
		expect(watermark.consumed([2])).toBe(2);
	});
});
