/**
 * Another agent's text in the transcript (`docs/design/arch/client.md` §4.3): control characters removed, so an
 * escape sequence cannot take effect, and a body quoted and collapsed to its first lines until expanded.
 * Shared by the entry card and the thread view.
 */
import type { Theme } from "@oh-my-pi/pi-coding-agent";

const COLLAPSED_LINES = 3;
/** A collapsed body cuts each shown line to this many characters, as omp's own peer card does. */
const COLLAPSED_WIDTH = 100;

/** Text from another agent, safe for the terminal: control characters (escape sequences, bells) are dropped. */
export function printable(text: string): string {
	return text.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, "");
}

/** The quoted body lines; collapsed to the first lines, each cut to a width, plus how many are left out. */
export function bodyLines(body: string | undefined, expanded: boolean, theme: Pick<Theme, "fg">): string[] {
	if (body === undefined) return [];
	const lines = printable(body.trimEnd().replace(/\t/g, "  ")).split("\n");
	const shown = expanded ? lines : lines.slice(0, COLLAPSED_LINES).map(line => (line.length > COLLAPSED_WIDTH ? `${line.slice(0, COLLAPSED_WIDTH - 1)}…` : line));
	const quoted = shown.map(line => `${theme.fg("mdQuoteBorder", "│")} ${line}`);
	const hidden = lines.length - shown.length;
	return hidden > 0 ? [...quoted, theme.fg("dim", `… ${hidden} more line${hidden === 1 ? "" : "s"}`)] : quoted;
}
