/**
 * The gate that keeps the extension off a server that is too old: it probes `GET /healthz` once per url
 * and refuses the server when its protocol is below the minimum this plugin needs
 * (`docs/design/arch/client.md` §8.1).
 *
 * A probe that fails (no answer, a network error) is not a refusal: the normal path fails and retries
 * as it does today.
 */
import type { FetchLike } from "../bus/client.ts";
import { probeProtocol } from "../bus/health.ts";
import { trimUrl } from "../session/stored-credentials.ts";
import { tooOld } from "../version/server-protocol.ts";

/** The verdict of a check: "ok" when the server may be used, the refusal text otherwise. */
export type ServerVerdict = "ok" | { refused: string };

export interface ServerGate {
	/** Checks the server's protocol; the probe of a url is cached. */
	check(url: string): Promise<ServerVerdict>;
	/** The last answered probe; undefined when none answered. */
	last(): { url: string; protocol: number | undefined } | undefined;
}

/** The gate of one session: it probes a url once and remembers what it answered. */
export function createServerGate(fetchFn: FetchLike): ServerGate {
	const probes = new Map<string, Promise<{ protocol: number | undefined } | undefined>>();
	let last: { url: string; protocol: number | undefined } | undefined;
	const check = async (url: string): Promise<ServerVerdict> => {
		const base = trimUrl(url);
		let probe = probes.get(base);
		if (!probe) {
			probe = probeProtocol(fetchFn, base).then(protocol => ({ protocol }), () => undefined);
			probes.set(base, probe);
		}
		const answered = await probe;
		if (!answered) {
			// The probe failed: that is not a refusal, and the next check probes again.
			probes.delete(base);
			return "ok";
		}
		last = { url: base, protocol: answered.protocol };
		const refusal = tooOld(answered.protocol, base);
		return refusal ? { refused: refusal } : "ok";
	};
	return { check, last: () => last };
}
