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
import { errorMessage } from "../bus/errors.ts";
import { type BusMessage, busMessage, noticeMessage } from "../render/details.ts";
import type { AckWatermark } from "./ack-watermark.ts";
import { isBusy, mechanismFor, type Mechanism, type SessionPhase } from "./delivery-class.ts";
import { createPostNotices, isPostNotice, NOTICE_WINDOW_MS, type PostNotices } from "./post-notices.ts";
import { createWaiters, type WaitFilter, type Waiters } from "./waiters.ts";

export interface Dispatcher {
	/** Receives an entry of the stream and hands it over to the session. */
	receive(entry: Entry): void;
	/** Resolves true when every entry up to `id` was handed over, false when `ms` ran out first. */
	handedUpTo(id: number, ms: number): Promise<boolean>;
	/** `bus_wait`: the held entries matching `filter`, removed from the hold, so omp never gets them. */
	takeHeld(filter: WaitFilter): Entry[];
	/** `bus_wait`: the next entry matching `filter`, given to the caller instead of omp; [] on timeout or abort. */
	waitNext(filter: WaitFilter, timeoutMs: number, signal?: AbortSignal): Promise<Entry[]>;
	/** The stream stopped: the held entries are dropped unhanded, so a replay delivers them again. */
	dropHeld(): void;
}

export interface DispatcherDeps {
	/** The phase of the session right now. */
	phase(): SessionPhase;
	/** The watermark of the acked entries. */
	watermark: AckWatermark;
	/** Hands the session message of an entry over to omp with the mechanism. */
	handOver(message: BusMessage, mechanism: Mechanism): void;
	/** Reports a held notice omp refused. */
	warn(message: string): void;
}

/** Where a new entry goes: to a waiting `bus_wait`, into the hold of its thread, or to omp. */
function createRouting(deps: DispatcherDeps, waitCalls: Waiters): { deliver(entry: Entry): void; notices: PostNotices } {
	const handOver = (entries: Entry[]): void => {
		const message = isPostNotice(entries[0]) ? noticeMessage(entries) : busMessage(entries[0]);
		deps.handOver(message, mechanismFor(parseDeliveryClass(entries[0].class), deps.phase()));
	};
	const flush = (entries: Entry[]): void => {
		try {
			handOver(entries);
		} catch (error) {
			deps.warn(errorMessage(error));
		}
	};
	const notices = createPostNotices({ windowMs: NOTICE_WINDOW_MS, flush });
	const deliver = (entry: Entry): void => {
		if (waitCalls.take(entry)) return;
		// A post joins its thread's held notice even when the session went idle meanwhile, so the posts stay in order.
		if (isPostNotice(entry) && (isBusy(deps.phase()) || notices.holds(entry))) notices.hold(entry);
		else handOver([entry]);
	};
	return { deliver, notices };
}

/** The `handedUpTo` calls: each resolves once the highest entry handed over reaches its id. */
function createProgress(): { advance(id: number): void; until(id: number, ms: number): Promise<boolean> } {
	let highest = 0;
	const waiters: { id: number; done: (delivered: boolean) => void; timer: ReturnType<typeof setTimeout> }[] = [];
	const remove = (waiter: (typeof waiters)[number]) => waiters.splice(waiters.indexOf(waiter), 1);
	return {
		advance(id) {
			highest = Math.max(highest, id);
			for (const waiter of waiters.filter(w => w.id <= highest)) {
				clearTimeout(waiter.timer);
				remove(waiter);
				waiter.done(true);
			}
		},
		until(id, ms) {
			if (highest >= id) return Promise.resolve(true);
			const { promise, resolve } = Promise.withResolvers<boolean>();
			const timer = setTimeout(() => {
				remove(waiter);
				resolve(false);
			}, ms);
			const waiter = { id, done: resolve, timer };
			waiters.push(waiter);
			return promise;
		},
	};
}

/** The dispatcher of one connection: it maps every entry to the mechanism its class asks for. */
export function createDispatcher(deps: DispatcherDeps): Dispatcher {
	const handedOver = new Set<number>();
	const waitCalls = createWaiters();
	const { deliver, notices } = createRouting(deps, waitCalls);
	const progress = createProgress();
	return {
		receive(entry) {
			deps.watermark.received(entry.id);
			// A reconnect replays the entries that were not acked; this connection already handed them over.
			if (handedOver.has(entry.id)) return;
			handedOver.add(entry.id);
			deliver(entry);
			progress.advance(entry.id);
		},
		takeHeld: notices.take,
		waitNext: waitCalls.wait,
		handedUpTo: progress.until,
		dropHeld() {
			for (const entry of notices.clear()) handedOver.delete(entry.id);
		},
	};
}
