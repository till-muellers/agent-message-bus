/**
 * The dispatcher: it receives an inbox entry and hands it over to omp with the mechanism its
 * delivery class and the session's phase ask for (`docs/design/arch/client.md` §3.2). An entry a
 * waiting `bus_wait` wants goes to that waiter instead (§3.4). While the session is busy, the posts of a
 * thread are held for a short window and handed over as one notice; `bus_wait` takes held entries (§3.5).
 *
 * The message (`render/details.ts`) carries the entry, so the request that consumes it acks it. A
 * reconnect replays the entries that were not acked; the ones this connection already handed over are skipped.
 */
import type { Entry } from "../bus/entry.ts";
import { parseDeliveryClass } from "../bus/entry.ts";
import { type BusMessage, busMessage, noticeMessage } from "../render/details.ts";
import type { AckWatermark } from "./ack-watermark.ts";
import { isBusy, mechanismFor, type Mechanism, type SessionPhase } from "./delivery-class.ts";
import { createPostNotices, isPostNotice, NOTICE_WINDOW_MS } from "./post-notices.ts";
import { createWaiters, type WaitFilter } from "./waiters.ts";

export interface Dispatcher {
	/** Receives an entry of the stream and hands it over to the session. */
	receive(entry: Entry): void;
	/** Resolves true when every entry up to `id` was handed over, false when `ms` ran out first. */
	handedUpTo(id: number, ms: number): Promise<boolean>;
	/** `bus_wait`: the held entries matching `filter`, removed from the hold, so omp never gets them. */
	takeHeld(filter: WaitFilter): Entry[];
	/** `bus_wait`: the next entry matching `filter`, given to the caller instead of omp; [] on timeout or abort. */
	waitNext(filter: WaitFilter, timeoutMs: number, signal?: AbortSignal): Promise<Entry[]>;
}

export interface DispatcherDeps {
	/** The phase of the session right now. */
	phase(): SessionPhase;
	/** The watermark of the acked entries. */
	watermark: AckWatermark;
	/** Hands the session message of an entry over to omp with the mechanism. */
	handOver(message: BusMessage, mechanism: Mechanism): void;
}

/** The dispatcher of one connection: it maps every entry to the mechanism its class asks for. */
export function createDispatcher(deps: DispatcherDeps): Dispatcher {
	const handedOver = new Set<number>();
	const waitCalls = createWaiters();
	const handOver = (entries: Entry[]): void => {
		const message = isPostNotice(entries[0]) ? noticeMessage(entries) : busMessage(entries[0]);
		deps.handOver(message, mechanismFor(parseDeliveryClass(entries[0].class), deps.phase()));
	};
	const notices = createPostNotices({ windowMs: NOTICE_WINDOW_MS, flush: handOver });
	const deliver = (entry: Entry): void => {
		if (waitCalls.take(entry)) return;
		if (isPostNotice(entry) && isBusy(deps.phase())) notices.hold(entry);
		else handOver([entry]);
	};
	let highest = 0;
	const waiters: { id: number; done: (delivered: boolean) => void; timer: ReturnType<typeof setTimeout> }[] = [];
	const settle = (): void => {
		for (const waiter of [...waiters]) {
			if (highest < waiter.id) continue;
			clearTimeout(waiter.timer);
			waiters.splice(waiters.indexOf(waiter), 1);
			waiter.done(true);
		}
	};
	return {
		receive(entry) {
			deps.watermark.received(entry.id);
			// A reconnect replays the entries that were not acked; this connection already handed them over.
			if (handedOver.has(entry.id)) return;
			handedOver.add(entry.id);
			highest = Math.max(highest, entry.id);
			deliver(entry);
			settle();
		},
		takeHeld: notices.take,
		waitNext: waitCalls.wait,
		handedUpTo(id, ms) {
			if (highest >= id) return Promise.resolve(true);
			const { promise, resolve } = Promise.withResolvers<boolean>();
			const waiter = {
				id,
				done: resolve,
				timer: setTimeout(() => {
					waiters.splice(waiters.indexOf(waiter), 1);
					resolve(false);
				}, ms),
			};
			waiters.push(waiter);
			return promise;
		},
	};
}
