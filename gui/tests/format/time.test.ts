import { describe, expect, test } from "bun:test";
import { formatTime } from "../../src/format/time.ts";

// bun test runs in UTC unless TZ is set, so local time is UTC here.
describe("times", () => {
	test("a server time is shown as local date and minute", () => {
		expect(formatTime("2026-11-04T20:10:59Z")).toBe("2026-11-04 20:10");
	});

	test("single digits are padded", () => {
		expect(formatTime("2026-01-02T03:04:00Z")).toBe("2026-01-02 03:04");
	});

	test("an unreadable time is shown as it came", () => {
		expect(formatTime("soon")).toBe("soon");
	});
});
