/**
 * Whether the sender of an entry can get a direct reply (`docs/design/arch/client.md` §4.2): only participants of a
 * space get messages there, and an admin may send without joining. The participants of each space are read with
 * `bus_list_participants` and kept; a sender the list does not name reads it again, so a newcomer is found. Lookups of
 * one space that overlap share one read. An entry whose sender is still missing is marked `senderNotJoined`, and its
 * reply hint offers no direct message.
 */
import type { Entry } from "../bus/entry.ts";
import { labelOf } from "../render/entry-label.ts";

export interface SenderReachDeps {
	/** The names of the space's participants now. */
	participants(space: string): Promise<string[]>;
}

export interface SenderReach {
	/** The entry, marked `senderNotJoined` when its reply hint would name a sender that is no participant. A failed lookup leaves it as it is. */
	mark(entry: Entry): Promise<Entry>;
	/** Drops the known participants: a new connection start. A read still running fills only the dropped cache. */
	forget(): void;
}

/** Only direct and topic messages offer a direct reply to their sender. */
function offersDirectReply(entry: Entry): boolean {
	const { about, ended } = labelOf(entry);
	return !ended && (about === "direct" || about === "topic");
}

export function createSenderReach(deps: SenderReachDeps): SenderReach {
	let known = new Map<string, Set<string>>();
	let reading = new Map<string, Promise<Set<string>>>();
	function read(space: string): Promise<Set<string>> {
		const running = reading.get(space);
		if (running) return running;
		const [cache, pending] = [known, reading];
		const names = deps
			.participants(space)
			.then(list => {
				const set = new Set(list);
				cache.set(space, set);
				return set;
			})
			.finally(() => pending.delete(space));
		pending.set(space, names);
		return names;
	}
	return {
		async mark(entry) {
			try {
				if (!offersDirectReply(entry) || known.get(entry.space)?.has(entry.from)) return entry;
				return (await read(entry.space)).has(entry.from) ? entry : { ...entry, senderNotJoined: true };
			} catch {
				return entry;
			}
		},
		forget() {
			known = new Map();
			reading = new Map();
		},
	};
}
