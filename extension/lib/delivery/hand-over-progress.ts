/** The `handedUpTo` calls of an inbox ledger: each resolves once the highest entry handed over reaches its id. */

export interface HandOverProgress {
	/** The entry `id` was handed over (or claimed, or skipped as a replay). */
	advance(id: number): void;
	/** Resolves true once every entry up to `id` was handed over, false when `ms` ran out first. */
	until(id: number, ms: number): Promise<boolean>;
}

interface Waiter {
	id: number;
	done(delivered: boolean): void;
	timer: ReturnType<typeof setTimeout>;
}

export function createHandOverProgress(): HandOverProgress {
	let highest = 0;
	const waiters: Waiter[] = [];
	const remove = (waiter: Waiter) => waiters.splice(waiters.indexOf(waiter), 1);
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
			const waiter: Waiter = { id, done: resolve, timer };
			waiters.push(waiter);
			return promise;
		},
	};
}
