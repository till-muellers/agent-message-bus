import { formatTime } from "../format/time.ts";
import { h, type VNode } from "../view/h.ts";

/** A server time as a <time> element: machine-readable datetime, local text. */
export function time(iso: string): VNode {
	return h("time", { datetime: iso }, formatTime(iso));
}
