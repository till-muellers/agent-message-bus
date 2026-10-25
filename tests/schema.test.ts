import { describe, expect, test } from "bun:test";
import { type JsonSchema, toZod } from "../extension/lib/schema.ts";
import { type FakeNode, fakeZ, toJson, z } from "./fakes.ts";

describe("toZod", () => {
	const schema: JsonSchema = {
		type: "object",
		description: "args",
		properties: {
			space: { type: "string", description: "space name" },
			upTo: { type: "integer" },
			critical: { type: "boolean" },
		},
		required: ["space", "upTo"],
		additionalProperties: false,
	};

	test("round-trips the server subset", () => {
		// Regression: omp's shim has no `z.strictObject`; the stand-in lacks it too, so calling it would throw here.
		expect("strictObject" in fakeZ).toBe(false);
		expect(toJson(toZod(schema, z) as unknown as FakeNode)).toEqual(schema);
	});

	test("optional fields keep their description", () => {
		const node = toZod({ type: "object", properties: { a: { type: "string", description: "d" } } }, z) as unknown as FakeNode;
		expect(node.shape?.a).toMatchObject({ type: "string", description: "d", isOptional: true });
		expect(node.isStrict).toBeUndefined();
	});

	test("rejects array", () => {
		expect(() => toZod({ type: "object", properties: { xs: { type: "array" } } }, z)).toThrow("unsupported schema type array");
	});
});
