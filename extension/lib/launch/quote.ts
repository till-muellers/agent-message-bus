/** Quoting for command lines that a shell (POSIX sh or PowerShell) will parse. */

export function shQuote(s: string): string {
	return `'${s.replaceAll("'", "'\\''")}'`;
}

export function psQuote(s: string): string {
	return `'${s.replaceAll("'", "''")}'`;
}

export function posixCommand(argv: readonly string[]): string {
	return argv.map(shQuote).join(" ");
}

export function powershellCommand(argv: readonly string[]): string {
	return `& ${argv.map(psQuote).join(" ")}`;
}
