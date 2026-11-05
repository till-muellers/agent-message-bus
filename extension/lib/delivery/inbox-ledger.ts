/**
 * The inbox ledger: the one owner of the delivery state of one connection start (`docs/design/arch/client.md` §3.3).
 *
 * Invariants:
 * - One ledger per connection start. A restart (session switch or branch, login, promote, accept) begins with a new
 *   ledger: it has handed nothing over, so the asides omp dropped with the old session are delivered again when the
 *   stream replays them, and ids the last start never saw consumed cannot hold the next acks.
 * - The stream delivers new ids in ascending order. A reconnect within the start replays the ids that were not acked;
 *   a replayed id is skipped and changes nothing.
 * - The ack `upTo` is the highest delivered id such that every delivered id up to it is consumed. An id that was
 *   consumed but never delivered on this start's stream holds nothing and is never acked.
 * - An id a forwarded `bus_wait` claimed counts as handed over and consumed; the stream skips it, and it is acked as
 *   soon as the stream delivers it.
 */
import type { Entry } from "../bus/entry.ts";
import { createRouting, type RoutingDeps } from "./entry-routing.ts";
import { createHandOverProgress } from "./hand-over-progress.ts";
import { createWaiters, type WaitFilter } from "./waiters.ts";

export interface InboxLedger {
	/** The start's stream delivered `entry`: a new id goes to a waiting `bus_wait`, into a held notice or to omp. */
	deliver(entry: Entry): void;
	/** `bus_wait`: the held entries matching `filter`, removed from the hold so omp never gets them; consumed. */
	takeHeld(filter: WaitFilter): Entry[];
	/** `bus_wait`: the next delivered entry matching `filter`, given to the caller instead of omp; consumed; [] on timeout or abort. */
	awaitEntry(filter: WaitFilter, ms: number, signal?: AbortSignal): Promise<Entry[]>;
	/** `bus_wait` forwarded to the server returned `entries`: the ones this start has not handed over are claimed and consumed; returns them. */
	claimFromServer(entries: readonly Entry[]): Entry[];
	/** A model request carried these ids; acks when that moves `upTo`. */
	consumed(ids: readonly number[]): void;
	/** The ack watermark: the highest id acked so far, 0 before the first ack. */
	upTo(): number;
	/** Resolves true when every entry up to `id` was handed over, false when `ms` ran out first. */
	handedUpTo(id: number, ms: number): Promise<boolean>;
	/** The start's stream stopped: the held entries are dropped unhanded and the waiting `bus_wait` calls return []. */
	stop(): void;
}

export interface InboxLedgerDeps extends RoutingDeps {
	/** Tells the server how far the session consumed the inbox. */
	ack(upTo: number): Promise<void>;
}

/** The ack watermark over the ids in the order the stream delivered them. */
interface Watermark {
	/** Records a delivered id; a replayed one changes nothing. */
	delivered(id: number): void;
	/** Records consumed ids; true when `upTo` moved. */
	consumed(ids: readonly number[]): boolean;
	upTo(): number;
}

function createWatermark(): Watermark {
	const order: number[] = [];
	const known = new Set<number>();
	const consumed = new Set<number>();
	let next = 0;
	let upTo = 0;
	return {
		delivered(id) {
			if (known.has(id)) return;
			known.add(id);
			order.push(id);
		},
		consumed(ids) {
			for (const id of ids) consumed.add(id);
			const before = next;
			while (next < order.length && consumed.has(order[next])) upTo = order[next++];
			return next !== before;
		},
		upTo: () => upTo,
	};
}

/** The ledger of one connection start. */
export function createInboxLedger(deps: InboxLedgerDeps): InboxLedger {
	const handedOver = new Set<number>();
	const claimed = new Set<number>();
	const watermark = createWatermark();
	const waiters = createWaiters();
	const { route, notices } = createRouting(deps, waiters);
	const progress = createHandOverProgress();
	const consumed = (ids: readonly number[]): void => {
		if (watermark.consumed(ids)) void deps.ack(watermark.upTo());
	};
	/** The entries `bus_wait` returns are consumed. */
	const taken = (entries: Entry[]): Entry[] => {
		if (entries.length > 0) consumed(entries.map(entry => entry.id));
		return entries;
	};
	return {
		deliver(entry) {
			watermark.delivered(entry.id);
			if (claimed.has(entry.id)) consumed([entry.id]);
			else if (!handedOver.has(entry.id)) {
				handedOver.add(entry.id);
				route(entry);
			}
			progress.advance(entry.id);
		},
		takeHeld: filter => taken(notices.take(filter)),
		awaitEntry: async (filter, ms, signal) => taken(await waiters.wait(filter, ms, signal)),
		claimFromServer(entries) {
			const unseen = entries.filter(entry => !handedOver.has(entry.id));
			for (const entry of unseen) {
				handedOver.add(entry.id);
				claimed.add(entry.id);
			}
			return taken(unseen);
		},
		consumed,
		upTo: () => watermark.upTo(),
		handedUpTo: progress.until,
		stop() {
			notices.clear();
			waiters.cancel();
		},
	};
}
