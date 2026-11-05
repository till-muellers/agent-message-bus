/**
 * The transcript card of an agent-bus message (`docs/design/arch/client.md` §4.3, `omp-rendering.feature`
 * rule 2): one card per entry, titled "<from> → you · <space> · <where>", the body quoted and collapsed to
 * its first lines until expanded, a critical entry in the warning tone. The card reads only the message's
 * details, so a resumed session shows the same card.
 */
import type { ExtensionAPI, MessageRenderer, Theme } from "@oh-my-pi/pi-coding-agent";
import type { Entry } from "../bus/entry.ts";
import { BUS_MESSAGE_TYPE, type BusMessageDetails } from "./details.ts";
import { labelOf } from "./entry-label.ts";
import { bodyLines, printable } from "./quote.ts";

/** The TUI components a card is built from (`pi.pi`). */
export type CardComponents = Pick<ExtensionAPI["pi"], "Container" | "Text">;
type Card = InstanceType<CardComponents["Container"]>;
/** The theme calls a card uses. */
export type CardTheme = Pick<Theme, "fg" | "bold">;

function where(e: Entry): string {
	const { about, ended } = labelOf(e);
	const title = printable((e.threadTitle ?? "").replace(/\s+/g, " ")).replaceAll('"', '\\"');
	switch (about) {
		case "direct":
			return "direct";
		case "topic":
			return ended ? `#${e.topic} closed` : `#${e.topic}`;
		case "thread":
			return ended ? `${e.blackboard} / ${e.threadId} removed` : `${e.blackboard} / ${e.threadId} "${title}"`;
		case "blackboard":
			return `${e.blackboard} removed`;
		case "other":
			return printable(e.type.replace(/\s+/g, " "));
	}
}

function title(e: Entry, theme: CardTheme): string {
	const label = e.critical ? ` · ${theme.bold("critical")}` : "";
	return theme.fg(e.critical ? "warning" : "accent", `${theme.bold(e.from)} → you · ${e.space} · ${where(e)}${label}`);
}

function card(pi: CardComponents, e: Entry, expanded: boolean, theme: CardTheme): Card {
	const container = new pi.Container();
	for (const line of [title(e, theme), ...bodyLines(e.body, expanded, theme)]) container.addChild(new pi.Text(line, 0, 0));
	return container;
}

/** The cards of a message's entries; undefined when it carries none, so omp shows its plain text. */
export function entryCard(pi: CardComponents, details: BusMessageDetails | undefined, expanded: boolean, theme: CardTheme): Card | undefined {
	const entries = details?.entries ?? [];
	if (entries.length === 0) return undefined;
	const cards = new pi.Container();
	for (const e of entries) cards.addChild(card(pi, e, expanded, theme));
	return cards;
}

/** Registers the card renderer for the extension's messages. */
export function registerEntryCard(pi: ExtensionAPI): void {
	const render: MessageRenderer<BusMessageDetails> = (message, { expanded }, theme) => entryCard(pi.pi, message.details, expanded, theme);
	pi.registerMessageRenderer(BUS_MESSAGE_TYPE, render);
}
