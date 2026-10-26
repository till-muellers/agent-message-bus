import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { parseDeliveryClass } from "../../extension/lib/bus/entry.ts";
import { holdable, mechanismFor, phaseOf, type SessionPhase } from "../../extension/lib/delivery/delivery-class.ts";

const PHASES: SessionPhase[] = ["interactive-busy", "interactive-idle", "oneshot-busy", "oneshot-idle"];

describe("the mechanism table", () => {
	test("every class in every phase has the mechanism of the design's table", () => {
		const table = Object.fromEntries(
			(["interrupt", "queue", "deferred"] as const).map(cls => [cls, PHASES.map(phase => mechanismFor(cls, phase))]),
		);
		expect(table).toEqual({
			interrupt: [
				{ deliverAs: "steer", triggerTurn: false },
				{ deliverAs: "steer", triggerTurn: true },
				{ deliverAs: "steer", triggerTurn: false },
				{ deliverAs: "nextTurn", triggerTurn: false },
			],
			queue: [
				{ deliverAs: "aside", triggerTurn: false },
				{ deliverAs: "aside", triggerTurn: false },
				{ deliverAs: "aside", triggerTurn: false },
				{ deliverAs: "nextTurn", triggerTurn: false },
			],
			deferred: [
				{ deliverAs: "nextTurn", triggerTurn: false },
				{ deliverAs: "nextTurn", triggerTurn: false },
				{ deliverAs: "nextTurn", triggerTurn: false },
				{ deliverAs: "nextTurn", triggerTurn: false },
			],
		});
	});
});

describe("phaseOf", () => {
	test("tui and rpc are interactive, print, json and an unknown mode are one-shot", () => {
		expect(phaseOf("tui", true)).toBe("interactive-idle");
		expect(phaseOf("tui", false)).toBe("interactive-busy");
		expect(phaseOf("rpc", true)).toBe("interactive-idle");
		expect(phaseOf("rpc", false)).toBe("interactive-busy");
		expect(phaseOf("print", true)).toBe("oneshot-idle");
		expect(phaseOf("print", false)).toBe("oneshot-busy");
		expect(phaseOf("json", true)).toBe("oneshot-idle");
		expect(phaseOf(undefined, true)).toBe("oneshot-idle");
		expect(phaseOf("something", false)).toBe("oneshot-busy");
	});
});

describe("parseDeliveryClass", () => {
	test("the wire values keep their class", () => {
		expect(parseDeliveryClass("interrupt")).toBe("interrupt");
		expect(parseDeliveryClass("queue")).toBe("queue");
		expect(parseDeliveryClass("deferred")).toBe("deferred");
	});

	test("a missing or unknown value is queue", () => {
		expect(parseDeliveryClass(undefined)).toBe("queue");
		expect(parseDeliveryClass(null)).toBe("queue");
		expect(parseDeliveryClass("something-new")).toBe("queue");
		expect(parseDeliveryClass(3)).toBe("queue");
	});
});

describe("holdable", () => {
	test("only queue entries may be held for coalescing", () => {
		expect(holdable("queue")).toBe(true);
		expect(holdable("interrupt")).toBe(false);
		expect(holdable("deferred")).toBe(false);
	});
});

describe("the delivery folder", () => {
	const dir = resolve(import.meta.dir, "..", "..", "extension", "lib", "delivery");

	test("only delivery-class.ts knows the delivery mechanisms", () => {
		const offenders = readdirSync(dir)
			.filter(file => file.endsWith(".ts") && file !== "delivery-class.ts")
			.filter(file => /"steer"|"aside"|"nextTurn"/.test(readFileSync(join(dir, file), "utf8")));
		expect(offenders).toEqual([]);
	});
});
