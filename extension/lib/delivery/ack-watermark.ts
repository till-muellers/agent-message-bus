/**
 * The watermark of the acked inbox entries: the highest id whose entries were all consumed by a
 * model request (`docs/design/arch/client.md` §3.3).
 *
 * The ids arrive on the stream in ascending order; an entry that is consumed before an older one
 * holds the watermark until the older one is consumed too.
 */
export interface AckWatermark {
	/** An entry arrived on the stream. */
	received(id: number): void;
	/** The ids a model request consumed; the new upTo when the watermark advanced, else undefined. */
	consumed(ids: readonly number[]): number | undefined;
}

/** The watermark of one connection. */
export function createAckWatermark(): AckWatermark {
	const received: number[] = [];
	const known = new Set<number>();
	const consumed = new Set<number>();
	let index = 0;
	return {
		received(id) {
			if (known.has(id)) return;
			known.add(id);
			received.push(id);
		},
		consumed(ids) {
			for (const id of ids) consumed.add(id);
			let upTo: number | undefined;
			while (index < received.length && consumed.has(received[index])) {
				upTo = received[index];
				index++;
			}
			return upTo;
		},
	};
}
