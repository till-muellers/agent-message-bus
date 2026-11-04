/** Reads the named field of the form that fired event and clears it, so the value stays nowhere. */
export function takeField(event: Event, name: string): string {
	const form = event.currentTarget;
	if (!(form instanceof HTMLFormElement)) return "";
	const field = form.elements.namedItem(name);
	if (!(field instanceof HTMLInputElement)) return "";
	const value = field.value;
	field.value = "";
	return value;
}
