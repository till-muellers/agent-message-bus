/**
 * Layout fitness: the extension's module folders and the import graph between them (`docs/design/arch/client.md` §1).
 *
 * Every module file under `extension/lib/` must live in one of the feature folders, the import graph must stay
 * acyclic, `bus/` and `process/` must stay leaves, `render/` must not reach into `delivery/` and only the wiring
 * root may see the whole graph. The allowed edges are the ones `client.md` §1.3 lists plus the edges the
 * not-yet-refactored code still has; each of those disappears with the slice that moves the module holding it.
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

/** The import edges between two feature folders that `client.md` §1.3 allows. */
const GRAPH_EDGES: Record<string, true> = {
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
};

/**
 * The edges the code still has and `client.md` §1.3 does not list yet: the credential types that several folders
 * read, the error wording in `bus/errors.ts` and the launchers behind `spawn/`. The slice that moves the module
 * holding each of them removes it.
 */
const TRANSITIONAL_EDGES: Record<string, true> = {
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

/** The test files that live in `tests/` itself: the shared doubles and the suite of the wiring root. */
const ROOT_TESTS: Record<string, true> = {
	"fakes.ts": true,
	"index.test.ts": true,
	"layout.test.ts": true,
};

/** The modules allowed to see the whole graph: the wiring root and, once it exists, the session. */
const ROOTS: Record<string, true> = {
	"index.ts": true,
	"session/session.ts": true,
};

/** A module of the extension. */
interface Module {
	/** Path relative to `extension/`, with `/` separators. */
	rel: string;
	/** The feature folder the module lives in; `""` while it sits flat in `lib/`. */
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
		const parts = relative(EXTENSION, path).split(sep);
		const folder = parts[0] === "lib" ? (parts.length > 2 ? parts[1] : "") : "index";
		return { rel: relative(EXTENSION, path).split(sep).join("/"), folder, path };
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

const MODULES = modules();
const BY_PATH = new Map(MODULES.map(module => [module.path, module]));
const EDGES = moduleEdges();

/** The folders that hold at least one module. */
function usedFolders(): string[] {
	return [...new Set(MODULES.map(module => module.folder))].filter(folder => folder !== "" && folder !== "index");
}

/** The folders a module reaches, directly or through another module. */
function reachableFolders(module: Module): Set<string> {
	const reached = new Set<string>();
	const seen = new Set<string>();
	const walk = (path: string): void => {
		for (const edge of EDGES) {
			if (edge.from !== path) continue;
			const target = BY_PATH.get(edge.to);
			if (!target || seen.has(target.path)) continue;
			seen.add(target.path);
			reached.add(target.folder);
			walk(target.path);
		}
	};
	walk(module.path);
	return reached;
}

describe("extension layout", () => {
	test("every module file lives in a feature folder", () => {
		const loose = MODULES.filter(module => module.folder !== "index" && !FOLDERS[module.folder]).map(module => module.rel);
		expect(loose).toEqual([]);
	});

	test("the import graph has no cycle", () => {
		const byFile = new Map<string, string[]>();
		for (const file of tsFiles(EXTENSION)) byFile.set(file, []);
		for (const edge of EDGES) byFile.get(edge.from)?.push(edge.to);
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
		expect([...inCycle].map(file => relative(EXTENSION, file).split(sep).join("/"))).toEqual([]);
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
			const key = `${from.folder}>${to.folder}`;
			return !GRAPH_EDGES[key] && !TRANSITIONAL_EDGES[key];
		}).map(edge => {
			const from = BY_PATH.get(edge.from);
			const to = BY_PATH.get(edge.to);
			return `${from?.folder} -> ${to?.folder} (${relative(EXTENSION, edge.from)})`;
		});
		expect(offenders).toEqual([]);
	});

	test("only the wiring root and the session see the whole graph", () => {
		const folders = usedFolders();
		const offenders = MODULES.filter(module => !ROOTS[module.rel])
			.filter(module => {
				const reached = reachableFolders(module);
				return folders.every(folder => reached.has(folder));
			})
			.map(module => module.rel);
		expect(offenders).toEqual([]);
	});
});

describe("test layout", () => {
	test("every test file in a folder tests the modules of that folder", () => {
		const offenders = tsFiles(TESTS)
			.map(file => relative(TESTS, file).split(sep).join("/"))
			.filter(rel => {
				if (!rel.includes("/")) return !ROOT_TESTS[rel];
				const [folder] = rel.split("/");
				if (!FOLDERS[folder]) return true;
				return !importsOf(join(ROOT, "tests", rel)).some(specifier => {
					const target = resolve(dirname(join(ROOT, "tests", rel)), specifier);
					return target.startsWith(join(LIB, folder) + sep) && target.endsWith(".ts");
				});
			});
		expect(offenders).toEqual([]);
	});
});
