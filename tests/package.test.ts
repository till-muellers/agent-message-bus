/**
 * Package fitness (`docs/design/arch/client.md` §9): the skill is retired. The plugin ships no `skills/` folder,
 * and no file outside the design record points at it; agents learn the bus from the context block and the guide.
 */
import { beforeAll, describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";

const ROOT = resolve(import.meta.dir, "..");
/** The design record describes the retirement, so it may name the skill. */
const DESIGN = "docs/design/";
const TEXT = /\.(md|ts|json|go|py|ya?ml|tmpl|html|feature)$/;
/** Spawning git on a loaded host can approach bun's default of 5 s per hook; git is stopped first, so its error shows. */
const GIT_TIMEOUT_MS = 30_000;
const HOOK_TIMEOUT_MS = 60_000;

/** Every tracked text file of the repo outside the design record (local and ignored files are not the repo's). */
let trackedTextFiles: string[] = [];

beforeAll(() => {
	const tracked = execFileSync("git", ["ls-files", "-z"], { cwd: ROOT, encoding: "utf8", timeout: GIT_TIMEOUT_MS }).split("\0");
	trackedTextFiles = tracked.filter(rel => TEXT.test(rel) && !rel.startsWith(DESIGN) && existsSync(join(ROOT, rel)));
}, HOOK_TIMEOUT_MS);

describe("the plugin package", () => {
	test("it ships no skill: files has no skills entry and the folder is gone", () => {
		const pkg = JSON.parse(readFileSync(join(ROOT, "package.json"), "utf8")) as { files: string[] };
		expect(pkg.files).not.toContain("skills");
		expect(existsSync(join(ROOT, "skills"))).toBe(false);
	});

	test("no file outside the design record refers to the retired skill", () => {
		const self = relative(ROOT, import.meta.path).split(sep).join("/");
		expect(trackedTextFiles).toContain(self);
		const offenders = trackedTextFiles.filter(rel => rel !== self && readFileSync(join(ROOT, rel), "utf8").includes("skills/agent-message-bus"));
		expect(offenders).toEqual([]);
	});
});
