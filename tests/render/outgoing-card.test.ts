/** The transcript card of a sent message (`omp-tools.feature` "The transcript shows a sent message as an outgoing card"). */
import { describe, expect, test } from "bun:test";
import type { CardTheme } from "../../extension/lib/render/entry-card.ts";
import { outgoingCard, type SendKind, sentMessage } from "../../extension/lib/render/outgoing-card.ts";
import { fakeComponents, type FakeComponent } from "../fakes.ts";

const theme: CardTheme = { fg: (color, text) => `<${color}>${text}</${color}>`, bold: text => `*${text}*` };
const texts = (c: FakeComponent): string[] => (c.kind === "text" ? [c.text ?? ""] : c.children.flatMap(texts));
const card = (kind: SendKind, args: Record<string, unknown>, expanded = false) =>
	texts(outgoingCard(fakeComponents, sentMessage(kind, args, "worker"), expanded, theme) as unknown as FakeComponent);
const quoted = (line: string) => `<mdQuoteBorder>│</mdQuoteBorder> ${line}`;

describe("the outgoing card", () => {
	test("a direct message reads <self> → <to> · <space> · direct with the body quoted", () => {
		expect(card("direct", { space: "alpha", to: "lead", body: "done" })).toEqual(["<accent>*worker* → lead · alpha · direct</accent>", quoted("done")]);
	});

	test("a topic message is addressed to the topic", () => {
		expect(card("topic", { space: "alpha", topic: "dev", body: "build is red" })).toEqual(["<accent>*worker* → #dev · alpha · topic</accent>", quoted("build is red")]);
	});

	test("a critical message is in the warning tone with its label", () => {
		expect(card("direct", { space: "alpha", to: "peer", body: "stop", critical: true })[0]).toBe("<warning>*worker* → peer · alpha · direct · *critical*</warning>");
	});

	test("a long body is collapsed until expanded, like an incoming card", () => {
		const args = { space: "alpha", to: "lead", body: "a\nb\nc\nd" };
		expect(card("direct", args).slice(1)).toEqual([quoted("a"), quoted("b"), quoted("c"), "<dim>… 1 more line</dim>"]);
		expect(card("direct", args, true).slice(1)).toHaveLength(4);
	});

	test("before the identity is known the sender is you; arguments still streaming in show as …", () => {
		const partial = outgoingCard(fakeComponents, sentMessage("direct", { space: "alpha" }, undefined), false, theme) as unknown as FakeComponent;
		expect(texts(partial)).toEqual(["<accent>*you* → … · alpha · direct</accent>"]);
	});

	test("control characters of the arguments are removed", () => {
		expect(card("direct", { space: "al\u0007pha", to: "le\u001b[2Jad", body: "x\u001b[31my" })).toEqual([
			"<accent>*worker* → le[2Jad · alpha · direct</accent>",
			quoted("x[31my"),
		]);
	});
});
