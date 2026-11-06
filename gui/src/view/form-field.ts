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

function fieldsOf(form: HTMLFormElement): Map<string, string> {
	const fields = new Map<string, string>();
	for (const [name, value] of new FormData(form)) if (typeof value === "string") fields.set(name, value);
	return fields;
}

function unchanged(form: HTMLFormElement, sent: ReadonlyMap<string, string>): boolean {
	const now = fieldsOf(form);
	return now.size === sent.size && [...sent].every(([name, value]) => now.get(name) === value);
}

/**
 * Hands every field of the form that fired event to act as text (a checked box is "on"), then resets the form once act
 * resolves true and the form still holds what was sent: a refused submit keeps what was typed, and so does text typed
 * while the answer was pending. A second submit while one is unanswered is ignored, so the kept input cannot be sent twice.
 */
export function submitForm(event: Event, act: (fields: ReadonlyMap<string, string>) => Promise<boolean>): void {
	const form = event.currentTarget;
	if (!(form instanceof HTMLFormElement) || submitting.has(form)) return;
	const sent = fieldsOf(form);
	submitting.add(form);
	void act(sent).then((went) => {
		submitting.delete(form);
		if (went && unchanged(form, sent)) form.reset();
	});
}
