import type { ShownOnce } from "../state/app-state.ts";
import { h, type VNode } from "../view/h.ts";

/**
 * An invite code or admin token, shown once: it is gone after the next navigation.
 * It takes the focus when it appears (another secret is another node, by key) and sticks to the top of the viewport.
 * While a confirmation is open it is hidden but stays in the page: a node that came back would take the focus again.
 */
export function shownOnceView(shown: ShownOnce, hidden = false): VNode {
	return h(
		"section",
		{ key: `shown-once-${shown.secret}`, class: "outcome", role: "status", "aria-label": shown.label, tabindex: -1, autofocus: true, hidden },
		h("p", null, `${shown.label}:`),
		h("code", null, shown.secret),
		h("p", null, "Copy it now: it cannot be shown again."),
	);
}
