/**
 * The time of an entry as the model reads it (`client.md` §4.2): the server's `createdAt` is RFC 3339 in UTC
 * with a fraction of variable length (`…:00Z` when it is zero); the model gets exactly three fraction
 * digits. String operations only, so the nanoseconds are not rounded through a `Date`.
 */
const RFC3339_UTC = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?Z$/;

/** `createdAt` as `YYYY-MM-DDTHH:MM:SS.mmmZ`, or undefined when it is not an RFC 3339 UTC time. */
export function isoMillis(createdAt: string): string | undefined {
	const match = RFC3339_UTC.exec(createdAt);
	if (match === null) return undefined;
	return `${match[1]}.${(match[2] ?? "").padEnd(3, "0").slice(0, 3)}Z`;
}
