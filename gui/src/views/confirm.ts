import type { PendingConfirm } from "../state/app-state.ts";
import { h, type VNode } from "../view/h.ts";

export interface ConfirmIntents {
	confirmed(confirm: PendingConfirm): void;
	dismiss(): void;
}

function question(confirm: PendingConfirm): { text: string; action: string } {
	if (confirm.kind === "delete-space") {
		return { text: `Delete space ${confirm.space} with its participants, topics, blackboards and invites?`, action: "Delete space" };
	}
	return { text: `Revoke ${confirm.role} ${confirm.name} from space ${confirm.space}?`, action: "Revoke" };
}

/** Asks before a revoke or a space deletion, naming the space, the target and its role. */
export function confirmView(confirm: PendingConfirm, intents: ConfirmIntents): VNode {
	const { text, action } = question(confirm);
	return h(
		"section",
		{ key: "confirm", role: "alertdialog", "aria-label": action },
		h("p", null, text),
		h("button", { type: "button", key: "confirm-yes", on: { click: () => intents.confirmed(confirm) } }, action),
		h("button", { type: "button", key: "confirm-no", on: { click: () => intents.dismiss() } }, "Cancel"),
	);
}
