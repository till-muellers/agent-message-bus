/** Converts the bus server's JSON-schema subset into a `pi.zod` schema. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";

export interface JsonSchema {
	type: string;
	description?: string;
	properties?: Record<string, JsonSchema>;
	required?: string[];
	additionalProperties?: boolean;
}

export type Zod = ExtensionAPI["zod"];
type ZodNode = Parameters<Zod["array"]>[0];

function convert(schema: JsonSchema, z: Zod): ZodNode {
	let node: ZodNode;
	switch (schema.type) {
		case "string":
			node = z.string();
			break;
		case "integer":
			node = z.number().int();
			break;
		case "boolean":
			node = z.boolean();
			break;
		case "object": {
			const required = new Set(schema.required ?? []);
			const shape: Record<string, ZodNode> = {};
			for (const [key, child] of Object.entries(schema.properties ?? {})) {
				const field = convert(child, z);
				shape[key] = required.has(key) ? field : field.optional();
			}
			// omp's `pi.zod` shim has no `z.strictObject`; `.strict()` on the object is the portable form.
			const object = z.object(shape);
			node = schema.additionalProperties === false ? object.strict() : object;
			break;
		}
		default:
			throw new Error(`unsupported schema type ${schema.type}`);
	}
	return schema.description ? node.describe(schema.description) : node;
}

/** Top-level tool parameters: always an object schema. */
export function toZod(schema: JsonSchema, z: Zod): ReturnType<Zod["object"]> {
	if (schema.type !== "object") throw new Error(`unsupported schema type ${schema.type}`);
	return convert(schema, z) as ReturnType<Zod["object"]>;
}
