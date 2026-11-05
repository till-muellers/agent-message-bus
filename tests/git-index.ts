/**
 * The paths tracked by git, read from the index file. Spawning `git ls-files` took seconds on a loaded host and
 * timed the package test out; reading the index takes under a millisecond.
 */
import { readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";

/** A SHA-1 index entry up to its path: ctime and mtime (seconds, nanoseconds), dev, ino, mode, uid, gid, size (4 bytes each), the 20-byte hash, the 2-byte flags. */
const ENTRY_HEAD = 62;
const EXTENDED_FLAG = 0x4000;

/** The git dir of the checkout at `root`: `.git` itself, or the directory that a worktree's `.git` file points to. */
function gitDir(root: string): string {
	const dotGit = join(root, ".git");
	if (statSync(dotGit).isDirectory()) return dotGit;
	return resolve(root, readFileSync(dotGit, "utf8").replace(/^gitdir:\s*/, "").trim());
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

/** The paths in the index of the checkout at `root` (index versions 2 to 4), relative with `/` separators. */
export function indexedPaths(root: string): string[] {
	const index = readFileSync(join(gitDir(root), "index"));
	const version = index.readUInt32BE(4);
	if (index.toString("latin1", 0, 4) !== "DIRC" || version < 2 || version > 4) throw new Error(`unsupported git index version ${version}`);
	const paths: string[] = [];
	let at = 12;
	let previous = Buffer.alloc(0);
	for (let left = index.readUInt32BE(8); left > 0; left--) {
		const entry = at;
		at += ENTRY_HEAD + (index.readUInt16BE(at + ENTRY_HEAD - 2) & EXTENDED_FLAG ? 2 : 0);
		const v4 = version === 4 ? readOffset(index, at) : undefined;
		const start = v4?.next ?? at;
		const end = index.indexOf(0, start);
		const name = index.subarray(start, end);
		previous = v4 ? Buffer.concat([previous.subarray(0, previous.length - v4.value), name]) : name;
		paths.push(previous.toString("utf8"));
		at = v4 ? end + 1 : entry + Math.ceil((end + 1 - entry) / 8) * 8;
	}
	rejectSplitIndex(index, at);
	return paths;
}
