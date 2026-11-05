/**
 * Package fitness (`docs/design/arch/client.md` §9): the skill is retired. The plugin ships no `skills/` folder,
 * and no file outside the design record points at it; agents learn the bus from the context block and the guide.
 */
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";

const ROOT = resolve(import.meta.dir, "..");
/** The design record describes the retirement, so it may name the skill. */
const DESIGN = "docs/design/";
const TEXT = /\.(md|ts|json|go|py|ya?ml|tmpl|html|feature)$/;
/** A SHA-1 index entry up to its flags: ctime, mtime, dev, ino, mode, uid, gid, size (4 bytes each), the hash, the flags. */
const ENTRY_HEAD = 62;
const EXTENDED_FLAG = 0x4000;

/** The git dir of the checkout: `.git` itself, or the directory that a worktree's `.git` file points to. */
function gitDir(): string {
	const dotGit = join(ROOT, ".git");
	if (statSync(dotGit).isDirectory()) return dotGit;
	return resolve(ROOT, readFileSync(dotGit, "utf8").replace(/^gitdir:\s*/, "").trim());
}

/** Git's offset varint (index v4): seven bits per byte, each continuation adding one before the shift. */
function readOffset(index: Buffer, at: number): { value: number; next: number } {
	let byte = index[at++];
	let value = byte & 0x7f;
	while (byte & 0x80) {
		byte = index[at++];
		value = ((value + 1) << 7) | (byte & 0x7f);
	}
	return { value, next: at };
}

/** A split index keeps entries in a second file, so reading this one alone would miss tracked files. */
function rejectSplitIndex(index: Buffer, at: number): void {
	for (; at + 8 <= index.length - 20; at += 8 + index.readUInt32BE(at + 4)) {
		if (index.toString("latin1", at, at + 4) === "link") throw new Error("split git index is not supported");
	}
}

/**
 * The paths in the git index (versions 2 to 4), read from the file: spawning `git ls-files` took seconds on a
 * loaded host and timed the test out, reading the index takes under a millisecond.
 */
function indexedPaths(): string[] {
	const index = readFileSync(join(gitDir(), "index"));
	const version = index.readUInt32BE(4);
	if (index.toString("latin1", 0, 4) !== "DIRC" || version < 2 || version > 4) throw new Error(`unsupported git index version ${version}`);
	const paths: string[] = [];
	let at = 12;
	for (let left = index.readUInt32BE(8); left > 0; left--) {
		const entry = at;
		at += ENTRY_HEAD + (index.readUInt16BE(at + ENTRY_HEAD - 2) & EXTENDED_FLAG ? 2 : 0);
		const v4 = version === 4 ? readOffset(index, at) : undefined;
		const start = v4?.next ?? at;
		const end = index.indexOf(0, start);
		const previous = paths.at(-1) ?? "";
		const prefix = v4 ? previous.slice(0, previous.length - v4.value) : "";
		paths.push(prefix + index.toString("utf8", start, end));
		at = v4 ? end + 1 : entry + Math.ceil((end + 1 - entry) / 8) * 8;
	}
	rejectSplitIndex(index, at);
	return paths;
}

/** Every tracked text file of the repo outside the design record (local and ignored files are not the repo's). */
function trackedTextFiles(): string[] {
	return indexedPaths().filter(rel => TEXT.test(rel) && !rel.startsWith(DESIGN) && existsSync(join(ROOT, rel)));
}

describe("the plugin package", () => {
	test("it ships no skill: files has no skills entry and the folder is gone", () => {
		const pkg = JSON.parse(readFileSync(join(ROOT, "package.json"), "utf8")) as { files: string[] };
		expect(pkg.files).not.toContain("skills");
		expect(existsSync(join(ROOT, "skills"))).toBe(false);
	});

	test("no file outside the design record refers to the retired skill", () => {
		const self = relative(ROOT, import.meta.path).split(sep).join("/");
		const files = trackedTextFiles();
		expect(files).toContain(self);
		const offenders = files.filter(rel => rel !== self && readFileSync(join(ROOT, rel), "utf8").includes("skills/agent-message-bus"));
		expect(offenders).toEqual([]);
	});
});
