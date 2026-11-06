import { submitForm } from "../view/form-field.ts";
import { h, type Handler, type VNode } from "../view/h.ts";
import type { Intents } from "./intents.ts";

/** A submit handler that hands the form's fields to act; the form resets only when act resolves true. */
export function onSubmit(act: (fields: ReadonlyMap<string, string>) => Promise<boolean>): Handler {
	return (event) => {
		event.preventDefault();
		submitForm(event, act);
	};
}

function field(fields: ReadonlyMap<string, string>, name: string): string {
	return fields.get(name) ?? "";
}

function labelled(id: string, label: string, control: VNode): VNode[] {
	return [h("label", { for: id }, label), control];
}

export function inviteForm(space: string, intents: Pick<Intents, "invite">): VNode {
	const submit = onSubmit((f) => intents.invite(space, field(f, "name"), field(f, "task")));
	return h(
		"form",
		{ key: "invite-form", on: { submit } },
		labelled("invite-name", "Invite member", h("input", { id: "invite-name", name: "name", autocomplete: "off", required: true })),
		labelled("invite-task", "Task (optional)", h("textarea", { id: "invite-task", name: "task", rows: 2 })),
		h("button", { type: "submit" }, "Create invite code"),
	);
}

/** Only leaders and admins may send critical messages; the caller leaves out topics it may not send to. */
export function sendForm(space: string, topics: readonly string[], lead: boolean, intents: Pick<Intents, "send">): VNode {
	const submit = onSubmit((f) => intents.send(space, field(f, "topic"), field(f, "body"), f.has("critical")));
	const options = topics.map((topic) => h("option", { key: topic, value: topic }, topic));
	return h(
		"form",
		{ key: "send-form", on: { submit } },
		labelled("send-topic", "Send to topic", h("select", { id: "send-topic", name: "topic", required: true }, options)),
		labelled("send-body", "Message", h("textarea", { id: "send-body", name: "body", rows: 2, required: true })),
		lead ? h("label", null, h("input", { type: "checkbox", name: "critical" }), " critical") : null,
		h("button", { type: "submit" }, "Send"),
	);
}

export function postForm(space: string, threadId: number, intents: Pick<Intents, "post">): VNode {
	const submit = onSubmit((f) => intents.post(space, threadId, field(f, "body")));
	return h(
		"form",
		{ key: "post-form", on: { submit } },
		labelled("post-body", "Reply", h("textarea", { id: "post-body", name: "body", rows: 3, required: true })),
		h("button", { type: "submit" }, "Post reply"),
	);
}
