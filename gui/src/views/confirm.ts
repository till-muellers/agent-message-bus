import type { PendingConfirm } from "../state/app-state.ts";
import { type Handler, h, type VNode } from "../view/h.ts";
import type { Intents } from "./intents.ts";

type ConfirmIntents = Pick<Intents, "confirmed" | "dismiss">;

function question(confirm: PendingConfirm): { text: string; action: string } {
	if (confirm.kind === "delete-space") {
		return { text: `Delete space ${confirm.space} with its participants, topics, blackboards and invites?`, action: "Delete space" };
	}
	return { text: `Revoke ${confirm.role} ${confirm.name} from space ${confirm.space}?`, action: "Revoke" };
}

/** Escape dismisses the confirmation (the alertdialog convention); the dialog has the focus when it opens. */
function dismissOnEscape(dismiss: () => void): Handler {
	return (event) => {
		if ("key" in event && event.key === "Escape") dismiss();
	};
}

/**
 * Asks before a revoke or a space deletion, naming the space, the target and its role.
 * Another confirmation is another node (the key), so it takes the focus when it appears; it sticks to the top of the viewport.
 */
export function confirmView(confirm: PendingConfirm, intents: ConfirmIntents): VNode {
	const { text, action } = question(confirm);
	return h(
		"section",
		{
			key: `confirm-${JSON.stringify(confirm)}`,
			class: "outcome",
			role: "alertdialog",
			"aria-label": action,
			tabindex: -1,
			autofocus: true,
			on: { keydown: dismissOnEscape(() => intents.dismiss()) },
		},
		h("p", null, text),
		h("button", { type: "button", key: "confirm-yes", on: { click: () => intents.confirmed(confirm) } }, action),
		h("button", { type: "button", key: "confirm-no", on: { click: () => intents.dismiss() } }, "Cancel"),
	);
}
