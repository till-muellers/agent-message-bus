import { describe, expect, test } from "bun:test";
import type { Joined } from "../extension/lib/accept.ts";
import { BusError } from "../extension/lib/bus/client.ts";
import { createInviteFlag } from "../extension/lib/invite-flag.ts";
import { CODE, URL_ } from "./fakes.ts";

const joined: Joined = { result: { message: "Joined space demo as member worker.", level: "info" }, task: "fix the bug" };

function host(flags: Record<string, unknown>, options: { sub?: boolean; storedUrl?: string; fail?: boolean } = {}) {
	const accepts: string[][] = [];
	const warnings: string[] = [];
	const redeem = createInviteFlag({
		flag: name => flags[name],
		isSub: () => options.sub ?? false,
		storedUrl: async () => options.storedUrl,
		accept: async (url, code) => {
			accepts.push([url, code]);
			if (options.fail) throw new BusError("unauthenticated", "invalid or expired invite code");
			return joined;
		},
		warn: message => void warnings.push(message),
	});
	return { redeem, accepts, warnings };
}

describe("--bus-accept", () => {
	test("is redeemed with --bus-url once; later calls redeem nothing", async () => {
		const { redeem, accepts } = host({ "bus-url": URL_, "bus-accept": CODE });
		expect(await redeem()).toEqual(joined);
		expect(await redeem()).toBeUndefined();
		expect(accepts).toEqual([[URL_, CODE]]);
	});

	test("falls back to the stored url", async () => {
		const { redeem, accepts } = host({ "bus-accept": CODE }, { storedUrl: "http://stored" });
		await redeem();
		expect(accepts).toEqual([["http://stored", CODE]]);
	});

	test("without any url it warns and redeems nothing", async () => {
		const { redeem, accepts, warnings } = host({ "bus-accept": CODE });
		expect(await redeem()).toBeUndefined();
		expect({ accepts, warnings }).toEqual({ accepts: [], warnings: ["--bus-accept needs --bus-url"] });
	});

	test("subagents ignore it, and so do an empty or non-string flag", async () => {
		for (const [flags, sub] of [
			[{ "bus-url": URL_, "bus-accept": CODE }, true],
			[{ "bus-url": URL_, "bus-accept": "" }, false],
			[{ "bus-url": URL_, "bus-accept": true }, false],
		] as const) {
			const { redeem, accepts } = host(flags, { sub });
			expect(await redeem()).toBeUndefined();
			expect(accepts).toEqual([]);
		}
	});

	test("a failed redemption is warned as <code>: <message> and not retried", async () => {
		const { redeem, accepts, warnings } = host({ "bus-url": URL_, "bus-accept": CODE }, { fail: true });
		expect(await redeem()).toBeUndefined();
		expect(await redeem()).toBeUndefined();
		expect({ accepts: accepts.length, warnings }).toEqual({ accepts: 1, warnings: ["unauthenticated: invalid or expired invite code"] });
	});
});
