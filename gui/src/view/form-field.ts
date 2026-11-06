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

/** The forms whose last submit has not been answered yet. */
const submitting = new WeakSet<HTMLFormElement>();

/**
 * Hands every field of the form that fired event to act as text (a checked box is "on"), then resets the form once act
 * resolves true: a refused submit keeps what was typed. A second submit while one is unanswered is ignored, so the
 * kept input cannot be sent twice.
 */
export function submitForm(event: Event, act: (fields: ReadonlyMap<string, string>) => Promise<boolean>): void {
	const form = event.currentTarget;
	if (!(form instanceof HTMLFormElement) || submitting.has(form)) return;
	const fields = new Map<string, string>();
	for (const [name, value] of new FormData(form)) if (typeof value === "string") fields.set(name, value);
	submitting.add(form);
	void act(fields).then((went) => {
		submitting.delete(form);
		if (went) form.reset();
	});
}
