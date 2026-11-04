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

/** Reads every field of the form that fired event as text (a checked box is "on") and resets the form. */
export function takeForm(event: Event): ReadonlyMap<string, string> {
	const form = event.currentTarget;
	if (!(form instanceof HTMLFormElement)) return new Map();
	const values = new Map<string, string>();
	for (const [name, value] of new FormData(form)) if (typeof value === "string") values.set(name, value);
	form.reset();
	return values;
}
