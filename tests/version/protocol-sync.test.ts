import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { MIN_SERVER_PROTOCOL } from "../../extension/lib/version/server-protocol.ts";

/** The server's protocol constant: `server/internal/server/protocol.go`. */
const PROTOCOL_GO = resolve(import.meta.dir, "..", "..", "server", "internal", "server", "protocol.go");

describe("the protocol of the server and the plugin", () => {
	test("the extension does not need a newer server than the server at HEAD", () => {
		const source = readFileSync(PROTOCOL_GO, "utf8");
		const match = source.match(/const Protocol = (\d+)/);
		expect(match).not.toBeNull();
		expect(Number(match?.[1])).toBeGreaterThanOrEqual(MIN_SERVER_PROTOCOL);
	});

	test("the extension needs protocol 2", () => {
		expect(MIN_SERVER_PROTOCOL).toBe(2);
	});
});
