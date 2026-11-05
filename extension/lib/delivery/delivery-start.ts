/**
 * What one connection start delivers with (`docs/design/arch/client.md` §3.3): a dispatcher and the ack watermark it
 * feeds. Every restart (session switch or branch, login, promote) begins with a fresh pair. The new dispatcher has
 * handed nothing over, so the asides omp dropped with the old session are delivered again when the stream replays
 * them; the new watermark has received nothing, so ids the last start never saw consumed cannot hold the next acks.
 */
import { createAckWatermark } from "./ack-watermark.ts";
import { type Consume, createConsumer } from "./consumer.ts";
import { createDispatcher, type Dispatcher, type DispatcherDeps } from "./dispatcher.ts";

export interface DeliveryStart {
	dispatcher: Dispatcher;
	/** Marks entries consumed; the ack follows this start's watermark. */
	consume: Consume;
}

export interface DeliveryStartDeps extends Omit<DispatcherDeps, "watermark" | "consume"> {
	/** Tells the server how far the session consumed the inbox. */
	ack(upTo: number): Promise<void>;
}

export function createDeliveryStart({ ack, ...dispatch }: DeliveryStartDeps): DeliveryStart {
	const watermark = createAckWatermark();
	const consume = createConsumer({ watermark, ack });
	return { dispatcher: createDispatcher({ ...dispatch, watermark, consume }), consume };
}
