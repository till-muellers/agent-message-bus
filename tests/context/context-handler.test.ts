/** The `context` handler: consumed entries, then the agent-bus-context block at the head (`client.md` §3.3, §5). */
import { describe, expect, test } from "bun:test";
import { createContextHandler } from "../../extension/lib/context/context-handler.ts";

const request = () => [
	{ role: "user", content: "hi" },
	{ role: "custom", customType: "agent-bus", details: { entryIds: [3] } },
];

function handler(text: { value: string | undefined }) {
	const consumed: number[][] = [];
	const handle = createContextHandler({ consume: ids => void consumed.push([...ids]), block: () => text.value });
	return { handle, consumed };
}

describe("the context handler", () => {
	test("it reports the consumed entries and puts the block before the original message objects", () => {
		const { handle, consumed } = handler({ value: "<agent-bus-context>\nX\n</agent-bus-context>" });
		const messages = request();
		const result = handle({ messages });
		expect(consumed).toEqual([[3]]);
		expect(result?.messages[0]).toMatchObject({ role: "custom", customType: "agent-bus-context", content: "<agent-bus-context>\nX\n</agent-bus-context>", display: false });
		expect(result?.messages.slice(1)).toHaveLength(2);
		result?.messages.slice(1).forEach((message, i) => expect(message).toBe(messages[i]));
	});

	test("the block message stays the same object while its text does not change", () => {
		const text = { value: "A" as string | undefined };
		const { handle } = handler(text);
		const first = handle({ messages: request() })?.messages[0];
		expect(handle({ messages: request() })?.messages[0]).toBe(first);
		text.value = "B";
		expect(handle({ messages: request() })?.messages[0]).toMatchObject({ content: "B" });
	});

	test("without a block the messages stay as they are", () => {
		const { handle, consumed } = handler({ value: undefined });
		expect(handle({ messages: request() })).toBeUndefined();
		expect(consumed).toEqual([[3]]);
	});
});
