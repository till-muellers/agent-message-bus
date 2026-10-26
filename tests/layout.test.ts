/**
 * Layout fitness: the extension's module folders and the import edges between them (`docs/design/arch/client.md` §1).
 *
 * Every module file under `extension/lib/` must live in one of the feature folders, the import graph must stay
 * acyclic, `bus/` and `process/` must stay leaves and `render/` must not reach into `delivery/`. The allowed
 * edges are the ones `client.md` §1.3 lists plus the edges the not-yet-refactored code still has; each of those
 * disappears with the slice that moves the module holding it.
 */
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";

const ROOT = resolve(import.meta.dir, "..");
const EXTENSION = join(ROOT, "extension");
const LIB = join(EXTENSION, "lib");
const TESTS = join(ROOT, "tests");

/** The feature folders of `lib/` (`client.md` §1.1). */
const FOLDERS: Record<string, true> = {
	bus: true,
	session: true,
	connection: true,
	tools: true,
	delivery: true,
	render: true,
	context: true,
	commands: true,
	"local-server": true,
	spawn: true,
	launch: true,
	process: true,
	version: true,
	shared: true,
};

/** The import edges `client.md` §1.3 allows between two feature folders. */
const ALLOWED_EDGES: Record<string, true> = {
	"commands>session": true,
	"commands>connection": true,
	"commands>local-server": true,
	"commands>spawn": true,
	"commands>version": true,
	"connection>session": true,
	"connection>tools": true,
	"connection>delivery": true,
	"connection>version": true,
	"connection>bus": true,
	"context>delivery": true,
	"context>bus": true,
	"context>session": true,
	"tools>delivery": true,
	"tools>render": true,
	"tools>bus": true,
	"delivery>render": true,
	"delivery>bus": true,
	"render>bus": true,
	"session>bus": true,
	"session>shared": true,
	"spawn>launch": true,
	"spawn>version": true,
	"spawn>bus": true,
	"spawn>process": true,
	"launch>process": true,
	"local-server>process": true,
	"local-server>bus": true,
	"version>process": true,
	"bus>shared": true,
	// Edges the code still has and `client.md` §1.3 does not list yet; the slice that moves the module holding
	// each of them removes it.
	"commands>bus": true,
	"commands>shared": true,
	"tools>session": true,
	"delivery>session": true,
	"delivery>connection": true,
	"local-server>session": true,
	"local-server>shared": true,
	"spawn>session": true,
	"spawn>tools": true,
};

/** The test files that live in `tests/` itself: the shared doubles and the root-level suites. */
const ROOT_TESTS: Record<string, true> = {
	"fakes.ts": true,
	"index.test.ts": true,
	"layout.test.ts": true,
};

/** The modules allowed to reach into many folders: the wiring root and, once it exists, the session. */
const ROOTS: Record<string, true> = {
	"index.ts": true,
	"session.ts": true,
};

/** A module of the extension. */
interface Module {
	/** Path relative to `extension/`, with `/` separators. */
	rel: string;
	/** The feature folder the module lives in; `index` for the wiring root, `""` while it sits flat in `lib/`. */
	folder: string;
	/** The file's absolute path. */
	path: string;
}

/** An import edge between two modules of the extension. */
interface Edge {
	from: string;
	to: string;
}

/** Every `.ts` file below `dir`, absolute. */
function tsFiles(dir: string): string[] {
	const found: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const path = join(dir, entry.name);
		if (entry.isDirectory()) found.push(...tsFiles(path));
		else if (entry.name.endsWith(".ts")) found.push(path);
	}
	return found;
}

/** The modules of the extension: `index.ts` plus everything under `lib/`. */
function modules(): Module[] {
	return tsFiles(EXTENSION).map(path => {
		const rel = relative(EXTENSION, path).split(sep).join("/");
		const folder = rel.startsWith("lib/") ? (rel.split("/")[1] ?? "") : "index";
		return { rel, folder, path };
	});
}

/** The relative import specifiers of a module. */
function importsOf(path: string): string[] {
	const source = readFileSync(path, "utf8");
	const specs: string[] = [];
	for (const match of source.matchAll(/(?:from|import)\s*\(?\s*["'](\.[^"']+)["']/g)) specs.push(match[1]);
	return specs;
}

/** The import edges between the extension's modules. */
function moduleEdges(): Edge[] {
	const edges: Edge[] = [];
	for (const file of tsFiles(EXTENSION)) {
		for (const spec of importsOf(file)) edges.push({ from: file, to: resolve(dirname(file), spec) });
	}
	return edges;
}

/** The modules that take part in an import cycle, if any. */
function cyclicModules(): string[] {
	const byFile = new Map<string, string[]>();
	for (const file of tsFiles(EXTENSION)) byFile.set(file, []);
	for (const edge of moduleEdges()) byFile.get(edge.from)?.push(edge.to);
	const state = new Map<string, "open" | "closed">();
	const inCycle = new Set<string>();
	const visit = (file: string, path: string[]): void => {
		state.set(file, "open");
		for (const next of byFile.get(file) ?? []) {
			if (!byFile.has(next)) continue;
			if (state.get(next) === "open") {
				for (const member of path.slice(path.indexOf(next))) inCycle.add(member);
				inCycle.add(next);
			} else if (!state.has(next)) visit(next, [...path, next]);
		}
		state.set(file, "closed");
	};
	for (const file of byFile.keys()) if (!state.has(file)) visit(file, [file]);
	return [...inCycle].map(file => relative(EXTENSION, file).split(sep).join("/"));
}

const MODULES = modules();
const BY_PATH = new Map(MODULES.map(module => [module.path, module]));
const EDGES = moduleEdges();

/** The folders a module reaches into, counted once each. */
function reachedFolders(module: Module): string[] {
	const folders = new Set<string>();
	for (const edge of EDGES) {
		if (edge.from !== module.path) continue;
		const target = BY_PATH.get(edge.to);
		if (target && target.folder !== module.folder) folders.add(target.folder);
	}
	return [...folders].sort();
}

describe("extension layout", () => {
	test("every module file lives in a feature folder", () => {
		const loose = MODULES.filter(module => module.rel !== "index.ts").filter(module => !FOLDERS[module.folder]).map(module => module.rel);
		expect(loose).toEqual([]);
	});

	test("the import graph has no cycle", () => {
		expect(cyclicModules()).toEqual([]);
	});

	test("bus and process are leaves: they import only themselves and shared", () => {
		const offenders = EDGES.filter(edge => {
			const from = BY_PATH.get(edge.from);
			const to = BY_PATH.get(edge.to);
			if (!from || !to) return false;
			if (from.folder !== "bus" && from.folder !== "process") return false;
			return to.folder !== from.folder && to.folder !== "shared";
		}).map(edge => `${relative(EXTENSION, edge.from)} -> ${relative(EXTENSION, edge.to)}`);
		expect(offenders).toEqual([]);
	});

	test("render never imports delivery", () => {
		const offenders = EDGES.filter(edge => BY_PATH.get(edge.from)?.folder === "render" && BY_PATH.get(edge.to)?.folder === "delivery").map(
			edge => `${relative(EXTENSION, edge.from)} -> ${relative(EXTENSION, edge.to)}`,
		);
		expect(offenders).toEqual([]);
	});

	test("no import edge leaves the allowed set", () => {
		const offenders = EDGES.filter(edge => {
			const from = BY_PATH.get(edge.from);
			const to = BY_PATH.get(edge.to);
			if (!from || !to || from.folder === to.folder) return false;
			if (from.folder === "index" || from.folder === "") return false;
			return !ALLOWED_EDGES[`${from.folder}>${to.folder}`];
		}).map(edge => {
			const from = BY_PATH.get(edge.from);
			const to = BY_PATH.get(edge.to);
			return `${from?.folder}/${relative(LIB, edge.from)} -> ${to?.folder}`;
		});
		expect(offenders).toEqual([]);
	});

	test("only the wiring root reaches into many folders", () => {
		const offenders = MODULES.filter(module => !ROOTS[module.rel.split("/").pop() ?? ""])
			.filter(module => reachedFolders(module).length > 5)
			.map(module => `${module.rel} reaches ${reachedFolders(module).join(", ")}`);
		expect(offenders).toEqual([]);
	});
});

describe("test layout", () => {
	test("every test file sits in the folder of the module it tests", () => {
		const libFiles = new Set(tsFiles(LIB));
		const offenders = tsFiles(TESTS)
			.map(file => relative(TESTS, file).split(sep).join("/"))
			.filter(rel => {
				if (!rel.includes("/")) return !ROOT_TESTS[rel];
				const [folder, name] = rel.split("/");
				if (!FOLDERS[folder]) return true;
				return !libFiles.has(join(LIB, folder, name.replace(/\.test\.ts$/, ".ts")));
			});
		expect(offenders).toEqual([]);
	});
});
