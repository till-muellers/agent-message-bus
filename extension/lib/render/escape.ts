/**
 * Escaping of untrusted text inside an envelope (`docs/design/arch/client.md` §4.2): a body or a
 * title cannot open or close an `agent-bus-entry` envelope or an `agent-bus-context` block, and
 * cannot pose as omp's harness (`<irc …>`, `<system-…>`), the same rule as omp's
 * `escapeHarnessTags` (`session/harness-tags.ts`). A forged `[agent-bus] … · CRITICAL` line in a
 * body needs no escaping: it stays inside the fence, plainly part of the body.
 */
const MARKER = /<(?=\s*\/?\s*(?:agent-bus-(?:entry|context)|irc|system-[a-z][a-z-]*)(?![\w-]))/gi;

/** `text` with every envelope, context and harness tag turned into plain text. */
export function escapeBody(text: string): string {
	return text.replace(MARKER, "&lt;");
}

/** A title for the header line: one line, its quotes escaped, its tags turned into text. */
export function escapeTitle(title: string): string {
	return escapeBody(title.replace(/\s+/g, " ").replaceAll('"', '\\"'));
}
