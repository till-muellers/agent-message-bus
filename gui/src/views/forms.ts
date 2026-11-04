import { takeForm } from "../view/form-field.ts";
import { h, type Handler, type VNode } from "../view/h.ts";

export interface FormIntents {
	invite(space: string, name: string, task: string): void;
	post(space: string, threadId: number, body: string): void;
	send(space: string, topic: string, body: string, critical: boolean): void;
}

/** A submit handler that reads and resets the form, then hands its fields to act. */
function onSubmit(act: (fields: ReadonlyMap<string, string>) => void): Handler {
	return (event) => {
		event.preventDefault();
		act(takeForm(event));
	};
}

function field(fields: ReadonlyMap<string, string>, name: string): string {
	return fields.get(name) ?? "";
}

function labelled(id: string, label: string, control: VNode): VNode[] {
	return [h("label", { for: id }, label), control];
}

export function inviteForm(space: string, intents: Pick<FormIntents, "invite">): VNode {
	const submit = onSubmit((f) => intents.invite(space, field(f, "name"), field(f, "task")));
	return h(
		"form",
		{ key: "invite-form", on: { submit } },
		labelled("invite-name", "Invite member", h("input", { id: "invite-name", name: "name", autocomplete: "off", required: true })),
		labelled("invite-task", "Task (optional)", h("textarea", { id: "invite-task", name: "task", rows: 2 })),
		h("button", { type: "submit" }, "Create invite code"),
	);
}

export function sendForm(space: string, topics: readonly string[], intents: Pick<FormIntents, "send">): VNode {
	const submit = onSubmit((f) => intents.send(space, field(f, "topic"), field(f, "body"), f.has("critical")));
	const options = topics.map((topic) => h("option", { key: topic, value: topic }, topic));
	return h(
		"form",
		{ key: "send-form", on: { submit } },
		labelled("send-topic", "Send to topic", h("select", { id: "send-topic", name: "topic", required: true }, options)),
		labelled("send-body", "Message", h("textarea", { id: "send-body", name: "body", rows: 2, required: true })),
		h("label", null, h("input", { type: "checkbox", name: "critical" }), " critical"),
		h("button", { type: "submit" }, "Send"),
	);
}

export function postForm(space: string, threadId: number, intents: Pick<FormIntents, "post">): VNode {
	const submit = onSubmit((f) => intents.post(space, threadId, field(f, "body")));
	return h(
		"form",
		{ key: "post-form", on: { submit } },
		labelled("post-body", "Reply", h("textarea", { id: "post-body", name: "body", rows: 3, required: true })),
		h("button", { type: "submit" }, "Post reply"),
	);
}
