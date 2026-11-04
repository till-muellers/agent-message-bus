import { describe, expect, test } from "bun:test";
import { Glob } from "bun";
import { join } from "node:path";

const SRC = join(import.meta.dir, "..", "src");
const RAW_HTML = /\b(innerHTML|outerHTML|insertAdjacentHTML)\b|\bdocument\.write(ln)?\b/;

async function sourceFiles(): Promise<string[]> {
	return Array.fromAsync(new Glob("**/*.ts").scan({ cwd: SRC }));
}

describe("no HTML from data", () => {
	test("the GUI sources never parse strings as HTML", async () => {
		const files = await sourceFiles();
		expect(files.length).toBeGreaterThan(0);
		const offenders: string[] = [];
		for (const file of files) {
			const text = await Bun.file(join(SRC, file)).text();
			if (RAW_HTML.test(text)) offenders.push(file);
		}
		expect(offenders).toEqual([]);
	});
});
