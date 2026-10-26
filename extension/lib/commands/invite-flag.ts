/**
 * `omp --bus-url <url> --bus-accept <code>`: redeems the invite code when the first top-level session starts. omp
 * re-parses the flags on every session switch, so the code is redeemed at most once per process, even when it fails.
 */
import type { Joined } from "./accept.ts";
import { errorMessage } from "../bus/errors.ts";

export interface InviteFlagHost {
	/** The value of a CLI flag registered by the extension. */
	flag(name: string): unknown;
	isSub(): boolean;
	/** The url of the stored credentials, used without `--bus-url`. */
	storedUrl(): Promise<string | undefined>;
	accept(url: string, code: string): Promise<Joined>;
	warn(message: string): void;
}

function nonEmpty(value: unknown): string | undefined {
	return typeof value === "string" && value ? value : undefined;
}

/** Resolves the invite it joined, else undefined (no flag, a subagent, already redeemed, no url, or a failure, which it warns). */
export function createInviteFlag(host: InviteFlagHost): () => Promise<Joined | undefined> {
	let used = false;
	return async () => {
		const code = nonEmpty(host.flag("bus-accept"));
		if (used || !code || host.isSub()) return undefined;
		used = true;
		const url = nonEmpty(host.flag("bus-url")) ?? (await host.storedUrl());
		if (!url) {
			host.warn("--bus-accept needs --bus-url");
			return undefined;
		}
		return host.accept(url, code).catch(error => {
			host.warn(errorMessage(error));
			return undefined;
		});
	};
}
