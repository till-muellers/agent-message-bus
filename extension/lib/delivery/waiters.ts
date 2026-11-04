/**
 * The `bus_wait` calls waiting in a dispatcher: the next entry a waiter's filter matches goes to that
 * waiter instead of omp (`docs/design/arch/client.md` §3.4).
 */
import type { Entry } from "../bus/entry.ts";

/** What a `bus_wait` waits for; the fields mean what they mean for the server's `bus_wait`. */
export interface WaitFilter {
	from?: string;
	threadId?: number;
	after?: number;
}

export interface Waiters {
	/** The next entry matching `filter`; [] when `ms` ran out or `signal` aborted first. */
	wait(filter: WaitFilter, ms: number, signal?: AbortSignal): Promise<Entry[]>;
	/** Gives `entry` to the oldest waiter it matches; false when none wants it. */
	take(entry: Entry): boolean;
}

interface Waiter {
	filter: WaitFilter;
	done(entries: Entry[]): void;
}

/** Whether `entry` is one a `bus_wait` with `filter` wants. */
export function matches(filter: WaitFilter, entry: Entry): boolean {
	if (filter.from !== undefined && entry.from !== filter.from) return false;
	if (filter.threadId !== undefined && entry.threadId !== filter.threadId) return false;
	return filter.after === undefined || entry.id > filter.after;
}

export function createWaiters(): Waiters {
	const waiters: Waiter[] = [];
	return {
		wait(filter, ms, signal) {
			if (signal?.aborted) return Promise.resolve([]);
			const { promise, resolve } = Promise.withResolvers<Entry[]>();
			const waiter: Waiter = {
				filter,
				done: entries => {
					clearTimeout(timer);
					signal?.removeEventListener("abort", nothing);
					waiters.splice(waiters.indexOf(waiter), 1);
					resolve(entries);
				},
			};
			const nothing = () => waiter.done([]);
			const timer = setTimeout(nothing, ms);
			signal?.addEventListener("abort", nothing, { once: true });
			waiters.push(waiter);
			return promise;
		},
		take(entry) {
			const waiter = waiters.find(w => matches(w.filter, entry));
			waiter?.done([entry]);
			return waiter !== undefined;
		},
	};
}
