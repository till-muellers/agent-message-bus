/**
 * The transcript card of a message this session sends (`omp-tools.feature` "The transcript shows a sent message as an
 * outgoing card"): the tool call renders as "<self> → <to> · <space> · <where>" with the body quoted like an incoming
 * card (`entry-card.ts`), a critical message in the warning tone. It is built from the call's arguments, which may still
 * be streaming in, so a missing one shows as "…"; the sender is "you" until the session's bus identity is known. Once
 * the arguments are complete, the sender is fixed for them, so a redraw after `/bus promote` or `/bus logout` still
 * names the agent that sent the message.
 */
import type { CardComponents, CardTheme } from "./entry-card.ts";
import { bodyLines, printable } from "./quote.ts";

/** What a send tool sends (`tools/tool-policy.ts` names the tools): a direct message or a topic message. */
export type SendKind = "direct" | "topic";

export interface OutgoingMessage {
	/** The session's bus agent; undefined while unknown. */
	from: string | undefined;
	to?: string;
	space?: string;
	/** What kind of message it is: `direct`, `topic`, `spawn`. */
	where: string;
	body?: string;
	critical?: boolean;
}

type Card = InstanceType<CardComponents["Container"]>;

/** A tool argument as card text: a string without its control characters, else undefined. */
export function argText(args: unknown, key: string): string | undefined {
	const value = (args as Record<string, unknown> | null)?.[key];
	return typeof value === "string" ? printable(value) : undefined;
}

/** The message the arguments of a send of this kind describe. */
export function sentMessage(kind: SendKind, args: unknown, from: string | undefined): OutgoingMessage {
	const common = { from, space: argText(args, "space"), body: argText(args, "body"), critical: (args as { critical?: unknown } | null)?.critical === true };
	if (kind === "direct") return { ...common, to: argText(args, "to"), where: "direct" };
	const topic = argText(args, "topic");
	return { ...common, to: topic === undefined ? undefined : `#${topic}`, where: "topic" };
}

/** The sender of a rendered call: the live identity while the arguments stream in, then the one seen first with them complete. */
export function fixedSender(self: () => string | undefined): (args: unknown, isPartial: boolean) => string | undefined {
	const fixed = new WeakMap<object, string>();
	return (args, isPartial) => {
		if (isPartial || typeof args !== "object" || args === null) return self();
		const from = fixed.get(args) ?? self();
		if (from !== undefined) fixed.set(args, from);
		return from;
	};
}

function title(m: OutgoingMessage, theme: CardTheme): string {
	const label = m.critical ? ` · ${theme.bold("critical")}` : "";
	const from = theme.bold(m.from === undefined ? "you" : printable(m.from));
	return theme.fg(m.critical ? "warning" : "accent", `${from} → ${m.to ?? "…"} · ${m.space ?? "…"} · ${m.where}${label}`);
}

export function outgoingCard(pi: CardComponents, message: OutgoingMessage, expanded: boolean, theme: CardTheme): Card {
	const card = new pi.Container();
	for (const line of [title(message, theme), ...bodyLines(message.body, expanded, theme)]) card.addChild(new pi.Text(line, 0, 0));
	return card;
}
