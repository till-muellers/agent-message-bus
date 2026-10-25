/**
 * Slash-command helpers shared by extension packages.
 *
 * `_shared/` is a support library folder, NOT an extension: it has no
 * `index.ts`, so omp never auto-loads it.
 */

/** What a command handler shows the user. */
export interface CommandResult {
	message: string;
	level: "info" | "warning" | "error";
}

/** Splits command arguments (a raw string or an omp argument array) into non-empty words. */
export function parseArgs(args: string | string[] | undefined): string[] {
	if (Array.isArray(args)) return args.map(String).filter(Boolean);
	return String(args ?? "").trim().split(/\s+/).filter(Boolean);
}
