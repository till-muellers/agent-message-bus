import { describe, expect, test } from "bun:test";
import { posixCommand, powershellCommand, psQuote, shQuote } from "../../extension/lib/launch/quote.ts";

describe("shQuote", () => {
	test("wraps plain and spaced values in single quotes", () => {
		expect(shQuote("a b")).toBe("'a b'");
		expect(shQuote("#x")).toBe("'#x'");
	});
	test("escapes embedded single quotes", () => {
		expect(shQuote("it's")).toBe("'it'\\''s'");
	});
});

describe("psQuote", () => {
	test("doubles embedded single quotes", () => {
		expect(psQuote("it's a #b")).toBe("'it''s a #b'");
	});
});

describe("commands", () => {
	test("posixCommand joins quoted argv", () => {
		expect(posixCommand(["omp", "--cwd", "/my dir"])).toBe("'omp' '--cwd' '/my dir'");
	});
	test("powershellCommand invokes quoted argv with &", () => {
		expect(powershellCommand(["omp", "C:\\a b"])).toBe("& 'omp' 'C:\\a b'");
	});
});
