/**
 * Package fitness (`docs/design/arch/client.md` §9): the skill is retired. The plugin ships no `skills/` folder,
 * and no file outside the design record points at it; agents learn the bus from the context block and the guide.
 */
import { describe, expect, test } from "bun:test";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";

const ROOT = resolve(import.meta.dir, "..");
/** Not searched: dependencies, generated or local state, and the design record, which describes the retirement. */
const SKIPPED = new Set(["node_modules", ".git", ".beads", "dist", join("docs", "design"), join("server", "internal", "web", "dist")]);
const TEXT = /\.(md|ts|json|go|py|ya?ml|tmpl|html|feature)$/;

/** Every text file of the repo outside the skipped folders, relative to the root. */
function textFiles(dir: string): string[] {
	return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
		const path = join(dir, entry.name);
		const rel = relative(ROOT, path);
		if (SKIPPED.has(entry.name) || SKIPPED.has(rel)) return [];
		if (entry.isDirectory()) return textFiles(path);
		return TEXT.test(entry.name) ? [rel.split(sep).join("/")] : [];
	});
}

describe("the plugin package", () => {
	test("it ships no skill: files has no skills entry and the folder is gone", () => {
		const pkg = JSON.parse(readFileSync(join(ROOT, "package.json"), "utf8")) as { files: string[] };
		expect(pkg.files).not.toContain("skills");
		expect(existsSync(join(ROOT, "skills"))).toBe(false);
	});

	test("no file outside the design record refers to the retired skill", () => {
		const self = relative(ROOT, import.meta.path).split(sep).join("/");
		const offenders = textFiles(ROOT).filter(rel => rel !== self && readFileSync(join(ROOT, rel), "utf8").includes("skills/agent-message-bus"));
		expect(offenders).toEqual([]);
	});
});
