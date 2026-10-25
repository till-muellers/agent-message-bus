/**
 * Slash-command helpers shared by the extension's modules.
 *
 * `lib/shared/` is a support library folder, NOT an extension: omp only loads
 * the entry declared in `package.json` (`extension/index.ts`).
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
