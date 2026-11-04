/** The server's agent guide, loaded once per connection (`client.md` §5, `contracts.md` §7.1). */
import { describe, expect, test } from "bun:test";
import { createGuide } from "../../extension/lib/context/guide.ts";

describe("the guide", () => {
	test("it holds the text the server served", async () => {
		const guide = createGuide(async () => "GUIDE", () => {});
		expect(guide.text()).toBeUndefined();
		await guide.load();
		expect(guide.text()).toBe("GUIDE");
	});

	test("a failed load leaves no guide and is logged once", async () => {
		const warnings: string[] = [];
		let fail = false;
		const guide = createGuide(
			async () => {
				if (fail) throw new Error("http_404: not found");
				return "GUIDE";
			},
			message => void warnings.push(message),
		);
		await guide.load();
		fail = true;
		await guide.load();
		expect(guide.text()).toBeUndefined();
		expect(warnings).toEqual(["guide not loaded: http_404: not found"]);
	});
});
