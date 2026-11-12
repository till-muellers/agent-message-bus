import { describe, expect, test } from "bun:test";
import type { Entry, EntryType } from "../../extension/lib/bus/entry.ts";
import { type CardTheme, entryCard } from "../../extension/lib/render/entry-card.ts";
import { busMessage } from "../../extension/lib/render/details.ts";
import { entry, fakeComponents, type FakeComponent } from "../fakes.ts";

/** Colors become tags, bold becomes stars, so the tests read the styling. */
const theme: CardTheme = { fg: (color, text) => `<${color}>${text}</${color}>`, bold: text => `*${text}*` };

const card = (e: Entry, expanded = false) => entryCard(fakeComponents, busMessage(e).details, expanded, theme) as unknown as FakeComponent;
const texts = (component: FakeComponent): string[] => component.children.flatMap(child => child.children.map(text => text.text ?? ""));

describe("the entry card", () => {
	test("a direct message is titled with the sender and the space and quotes the body", () => {
		expect(texts(card(entry(1, { from: "peer", space: "alpha", body: "hi" })))).toEqual(["<accent>*peer* → you · alpha · direct</accent>", "<mdQuoteBorder>│</mdQuoteBorder> hi"]);
	});

	test("the card names where the entry came from", () => {
		const cases: [Partial<Entry>, string][] = [
			[{ type: "topic_message", topic: "dev" }, "#dev"],
			[{ type: "topic_closed", topic: "dev" }, "#dev closed"],
			[{ type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "Rollout" }, 'plan / 7 "Rollout"'],
			[{ type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "Rollout", postId: 27 }, 'plan / 7 "Rollout" · post 27'],
			[{ type: "thread_removed", blackboard: "plan", threadId: 7 }, "plan / 7 removed"],
			[{ type: "blackboard_removed", blackboard: "plan", body: undefined }, "plan removed"],
			[{ type: "space_renamed" as string as EntryType }, "space_renamed"],
		];
		for (const [fields, where] of cases) expect(texts(card(entry(1, fields)))[0]).toBe(`<accent>*lead* → you · demo · ${where}</accent>`);
	});

	test("the body is collapsed to its first three lines until expanded", () => {
		const e = entry(1, { body: "one\ntwo\nthree\nfour\nfive" });
		expect(texts(card(e)).slice(1)).toEqual([
			"<mdQuoteBorder>│</mdQuoteBorder> one",
			"<mdQuoteBorder>│</mdQuoteBorder> two",
			"<mdQuoteBorder>│</mdQuoteBorder> three",
			"<dim>… 2 more lines</dim>",
		]);
		expect(texts(card(e, true))).toHaveLength(6);
	});

	test("a critical entry gets the warning tone and the label critical", () => {
		expect(texts(card(entry(1, { critical: true, body: "stop" })))[0]).toBe("<warning>*lead* → you · demo · direct · *critical*</warning>");
	});

	test("a thread title stays on one line and terminal control characters in the text are dropped", () => {
		const e = entry(1, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: "a\nb", body: "x\u001b[2Jy\u0007" });
		expect(texts(card(e))).toEqual(['<accent>*lead* → you · demo · plan / 7 "a b"</accent>', "<mdQuoteBorder>│</mdQuoteBorder> x[2Jy"]);
	});

	test("a collapsed card cuts long lines and drops the trailing newline; quotes in a thread title are escaped", () => {
		const long = "x".repeat(150);
		expect(texts(card(entry(1, { body: `${long}\n` })))).toEqual(["<accent>*lead* → you · demo · direct</accent>", `<mdQuoteBorder>│</mdQuoteBorder> ${"x".repeat(99)}…`]);
		expect(texts(card(entry(1, { body: long }), true))[1]).toBe(`<mdQuoteBorder>│</mdQuoteBorder> ${long}`);
		const forged = entry(1, { type: "thread_post", blackboard: "plan", threadId: 7, threadTitle: 'x" · critical' });
		expect(texts(card(forged))[0]).toBe('<accent>*lead* → you · demo · plan / 7 "x\\" · critical"</accent>');
	});

	test("every entry of a message gets its own card; a message without entries gets none", () => {
		const details = { entryIds: [1, 2], entries: [entry(1), entry(2)] };
		expect((entryCard(fakeComponents, details, false, theme) as unknown as FakeComponent).children).toHaveLength(2);
		expect(entryCard(fakeComponents, undefined, false, theme)).toBeUndefined();
		expect(entryCard(fakeComponents, { entryIds: [], entries: [] }, false, theme)).toBeUndefined();
	});
});
