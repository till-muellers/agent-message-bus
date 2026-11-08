/** The transcript view of a bus_read_thread result (`omp-blackboards.feature` rule 5). */
import { describe, expect, test } from "bun:test";
import type { CardTheme } from "../../extension/lib/render/entry-card.ts";
import { threadView } from "../../extension/lib/render/thread-view.ts";
import { fakeComponents, type FakeComponent } from "../fakes.ts";

const theme: CardTheme = { fg: (color, text) => `<${color}>${text}</${color}>`, bold: text => `*${text}*` };

function thread(posts: number) {
	return {
		id: 7,
		blackboard: "plan",
		title: "design",
		createdBy: "peer",
		subscribed: true,
		posts: Array.from({ length: posts }, (_, i) => ({ id: i + 1, author: i % 2 ? "lead" : "peer", body: `post ${i + 1}`, createdAt: `2026-11-0${i + 1}T10:00:00Z` })),
	};
}

const result = (value: unknown) => ({ content: [{ type: "text" as const, text: JSON.stringify(value) }] });
const view = (value: unknown, expanded = false) => threadView(fakeComponents, result(value), expanded, theme) as unknown as FakeComponent;
/** Every text of the tree, depth first. */
const texts = (c: FakeComponent): string[] => (c.kind === "text" ? [c.text ?? ""] : c.children.flatMap(texts));

describe("the thread view", () => {
	test("the thread title comes first, then each post with its author and time and its body quoted", () => {
		expect(texts(view(thread(2)))).toEqual([
			"<accent>*design*</accent> <dim>plan / 7 · 2 posts</dim>",
			"*peer* <dim>2026-11-01T10:00:00Z</dim>",
			"<mdQuoteBorder>│</mdQuoteBorder> post 1",
			"*lead* <dim>2026-11-02T10:00:00Z</dim>",
			"<mdQuoteBorder>│</mdQuoteBorder> post 2",
		]);
	});

	test("a long thread is collapsed to its newest posts until expanded", () => {
		const collapsed = texts(view(thread(5)));
		expect(collapsed[1]).toBe("<dim>… 2 older posts</dim>");
		expect(collapsed.filter(line => line.includes("│"))).toEqual(["post 3", "post 4", "post 5"].map(body => `<mdQuoteBorder>│</mdQuoteBorder> ${body}`));
		expect(texts(view(thread(5), true)).filter(line => line.includes("│"))).toHaveLength(5);
	});

	test("a long post is collapsed like an entry card until expanded", () => {
		const long = { ...thread(1), posts: [{ id: 1, author: "peer", body: "a\nb\nc\nd\ne", createdAt: "t" }] };
		expect(texts(view(long)).slice(2)).toEqual(["<mdQuoteBorder>│</mdQuoteBorder> a", "<mdQuoteBorder>│</mdQuoteBorder> b", "<mdQuoteBorder>│</mdQuoteBorder> c", "<dim>… 2 more lines</dim>"]);
		expect(texts(view(long, true)).slice(2)).toHaveLength(5);
	});

	test("control characters of the title, the author and the bodies are removed", () => {
		const hostile = { ...thread(0), title: "de\u001b[31msign", posts: [{ id: 1, author: "pe\u0007er", body: "x\u001b[2Jy", createdAt: "t" }] };
		expect(texts(view(hostile))).toEqual(["<accent>*de[31msign*</accent> <dim>plan / 7 · 1 post</dim>", "*peer* <dim>t</dim>", "<mdQuoteBorder>│</mdQuoteBorder> x[2Jy"]);
	});

	test("a result that is not a thread is shown as its text", () => {
		expect(texts(view({ unexpected: true }))).toEqual([JSON.stringify({ unexpected: true })]);
		const error = { content: [{ type: "text" as const, text: "not_found: no thread 9" }] };
		expect(texts(threadView(fakeComponents, error, false, theme) as unknown as FakeComponent)).toEqual(["not_found: no thread 9"]);
	});
});
