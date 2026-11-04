/**
 * The transcript card of an agent-bus message (`docs/design/arch/client.md` §4.3, `omp-rendering.feature`
 * rule 2): one card per entry, titled "<from> → you · <space> · <where>", the body quoted and collapsed to
 * its first lines until expanded, a critical entry in the warning tone. The card reads only the message's
 * details, so a resumed session shows the same card.
 */
import type { ExtensionAPI, MessageRenderer, Theme } from "@oh-my-pi/pi-coding-agent";
import type { Entry } from "../bus/entry.ts";
import { BUS_MESSAGE_TYPE, type BusMessageDetails } from "./details.ts";

/** The TUI components a card is built from (`pi.pi`). */
export type CardComponents = Pick<ExtensionAPI["pi"], "Container" | "Text">;
type Card = InstanceType<CardComponents["Container"]>;
/** The theme calls a card uses. */
export type CardTheme = Pick<Theme, "fg" | "bold">;

const COLLAPSED_LINES = 3;
/** A collapsed card cuts each shown line to this many characters, as omp's own peer card does. */
const COLLAPSED_WIDTH = 100;

/** Text from another agent, safe for the terminal: control characters (escape sequences, bells) are dropped. */
function printable(text: string): string {
	return text.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, "");
}

function where(e: Entry): string {
	const title = printable((e.threadTitle ?? "").replace(/\s+/g, " ")).replaceAll('"', '\\"');
	switch (e.type) {
		case "direct_message":
			return "direct";
		case "topic_message":
			return `#${e.topic}`;
		case "topic_closed":
			return `#${e.topic} closed`;
		case "thread_post":
			return `${e.blackboard} / ${e.threadId} "${title}"`;
		case "thread_removed":
			return `${e.blackboard} / ${e.threadId} removed`;
		case "blackboard_removed":
			return `${e.blackboard} removed`;
	}
}

function title(e: Entry, theme: CardTheme): string {
	const label = e.critical ? ` · ${theme.bold("critical")}` : "";
	return theme.fg(e.critical ? "warning" : "accent", `${theme.bold(e.from)} → you · ${e.space} · ${where(e)}${label}`);
}

/** The quoted body lines; collapsed to the first lines, each cut to a width, plus how many are left out. */
function bodyLines(body: string | undefined, expanded: boolean, theme: CardTheme): string[] {
	if (body === undefined) return [];
	const lines = printable(body.trimEnd().replace(/\t/g, "  ")).split("\n");
	const shown = expanded ? lines : lines.slice(0, COLLAPSED_LINES).map(line => (line.length > COLLAPSED_WIDTH ? `${line.slice(0, COLLAPSED_WIDTH - 1)}…` : line));
	const quoted = shown.map(line => `${theme.fg("mdQuoteBorder", "│")} ${line}`);
	const hidden = lines.length - shown.length;
	return hidden > 0 ? [...quoted, theme.fg("dim", `… ${hidden} more line${hidden === 1 ? "" : "s"}`)] : quoted;
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
