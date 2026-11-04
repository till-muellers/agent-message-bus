function pad(n: number): string {
	return String(n).padStart(2, "0");
}

/** A server time (RFC 3339) as local "YYYY-MM-DD HH:MM"; an unreadable value as it came. */
export function formatTime(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return iso;
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
