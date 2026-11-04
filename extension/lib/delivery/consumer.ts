/**
 * Consumption to ack: the entries a model request carried or `bus_wait` returned are consumed; when that moves the
 * watermark, the acker tells the server (`docs/design/arch/client.md` §3.3).
 */
import type { AckWatermark } from "./ack-watermark.ts";

export interface ConsumerDeps {
	watermark: AckWatermark;
	ack(upTo: number): Promise<void>;
}

/** Marks entry ids consumed and acks when the watermark advanced. */
export type Consume = (ids: readonly number[]) => void;

export function createConsumer(deps: ConsumerDeps): Consume {
	return ids => {
		const upTo = deps.watermark.consumed(ids);
		if (upTo !== undefined) void deps.ack(upTo);
	};
}
