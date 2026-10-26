/**
 * The bus protocol version of a server: `GET /healthz` answers `ok` and the header
 * `Agent-Bus-Protocol: N` (`docs/design/arch/contracts.md` §6). A server without the header speaks
 * no protocol; the extension refuses it before it redeems an invite or activates a tool
 * (`docs/design/arch/client.md` §8.1).
 */
import type { FetchLike } from "./client.ts";

/** The header the server reports its protocol version in. */
export const PROTOCOL_HEADER = "Agent-Bus-Protocol";

/** The protocol version the server reports; undefined when it reports none. Rejects when the probe fails. */
export async function probeProtocol(fetchFn: FetchLike, url: string): Promise<number | undefined> {
	const response = await fetchFn(`${url.replace(/\/+$/, "")}/healthz`, { method: "GET", headers: {} });
	if (!response.ok) throw new Error(`health probe: ${response.status} ${response.statusText}`);
	const header = response.headers.get(PROTOCOL_HEADER);
	if (!header) return undefined;
	const protocol = Number.parseInt(header, 10);
	return Number.isNaN(protocol) ? undefined : protocol;
}
