import { afterEach, describe, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { buildGui } from "../scripts/build.ts";

const outDirs: string[] = [];

function tempOut(): string {
	const dir = mkdtempSync(join(tmpdir(), "amb-gui-build-"));
	outDirs.push(dir);
	return dir;
}

afterEach(() => {
	for (const dir of outDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

describe("the GUI build", () => {
	test("index.html references the hashed main bundle it wrote", async () => {
		const out = tempOut();
		await buildGui(out);
		const html = readFileSync(join(out, "index.html"), "utf8");
		const src = html.match(/<script type="module" src="\/web\/assets\/(main-[^"]+\.js)"><\/script>/)?.[1];
		expect(src).toBeDefined();
		expect(existsSync(join(out, "assets", src ?? ""))).toBe(true);
	});

	test("index.html links the hashed stylesheet it wrote", async () => {
		const out = tempOut();
		await buildGui(out);
		const html = readFileSync(join(out, "index.html"), "utf8");
		const href = html.match(/<link rel="stylesheet" href="\/web\/assets\/(style-[^"]+\.css)">/)?.[1];
		expect(href).toBeDefined();
		expect(existsSync(join(out, "assets", href ?? ""))).toBe(true);
	});

	test("stale assets of an earlier build are removed", async () => {
		const out = tempOut();
		mkdirSync(join(out, "assets"));
		writeFileSync(join(out, "assets", "main-stale.js"), "");
		await buildGui(out);
		const files = readdirSync(join(out, "assets"));
		expect(files).not.toContain("main-stale.js");
		expect(files).toHaveLength(2);
	});

	test("files next to the assets, such as the embed placeholder, are kept", async () => {
		const out = tempOut();
		writeFileSync(join(out, ".gitkeep"), "");
		await buildGui(out);
		expect(existsSync(join(out, ".gitkeep"))).toBe(true);
	});

	test("the command line builds into the directory given by --out", () => {
		const out = tempOut();
		const run = Bun.spawnSync([process.execPath, join(import.meta.dir, "..", "scripts", "build.ts"), "--out", out]);
		expect(run.exitCode).toBe(0);
		expect(existsSync(join(out, "index.html"))).toBe(true);
	});
});
