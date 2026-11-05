/**
 * Where a new inbox entry goes (`docs/design/arch/client.md` §3.2–§3.5): to a waiting `bus_wait`, into the held notice
 * of its thread while the session is busy, or to omp with the mechanism its delivery class and the session's phase ask for.
 */
import type { Entry } from "../bus/entry.ts";
import { parseDeliveryClass } from "../bus/entry.ts";
import { errorMessage } from "../bus/errors.ts";
import { type BusMessage, busMessage, noticeMessage } from "../render/details.ts";
import { isBusy, mechanismFor, type Mechanism, type SessionPhase } from "./delivery-class.ts";
import { createPostNotices, isPostNotice, NOTICE_WINDOW_MS, type PostNotices } from "./post-notices.ts";
import type { Waiters } from "./waiters.ts";

export interface RoutingDeps {
	/** The phase of the session right now. */
	phase(): SessionPhase;
	/** Hands the session message of one or more entries over to omp with the mechanism. */
	handOver(message: BusMessage, mechanism: Mechanism): void;
	/** Reports a held notice omp refused. */
	warn(message: string): void;
}

export interface Routing {
	/** Routes an entry that was not handed over before. */
	route(entry: Entry): void;
	/** The held notices of the busy session's threads. */
	notices: PostNotices;
}

export function createRouting(deps: RoutingDeps, waiters: Waiters): Routing {
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
	const route = (entry: Entry): void => {
		if (waiters.take(entry)) return;
		// A post joins its thread's held notice even when the session went idle meanwhile, so the posts stay in order.
		if (isPostNotice(entry) && (isBusy(deps.phase()) || notices.holds(entry))) notices.hold(entry);
		else handOver([entry]);
	};
	return { route, notices };
}
