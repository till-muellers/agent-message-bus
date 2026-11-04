import type { ShownOnce } from "../state/app-state.ts";
import { h, type VNode } from "../view/h.ts";

/** An invite code or admin token, shown once: it is gone after the next navigation. */
export function shownOnceView(shown: ShownOnce): VNode {
	return h(
		"section",
		{ key: "shown-once", role: "status" },
		h("p", null, `${shown.label}:`),
		h("code", null, shown.secret),
		h("p", null, "Copy it now: it cannot be shown again."),
	);
}
