/**
 * Escaping of untrusted text inside an envelope (`docs/design/arch/client.md` §4.2): a body or a
 * title cannot open or close an `agent-bus-entry` envelope or an `agent-bus-context` block, the
 * same idea as omp's `escapeHarnessTags`. A forged `[agent-bus] … · CRITICAL` line needs no
 * escaping: it stays inside the fence, plainly part of the body.
 */
const MARKER = /<(\/?agent-bus-(?:entry|context))/gi;

/** `text` with every envelope and context marker turned into plain text. */
export function escapeBody(text: string): string {
	return text.replace(MARKER, "&lt;$1");
}
