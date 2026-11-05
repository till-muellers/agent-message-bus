/**
 * Layout fitness of `gui/src` (`docs/design/arch/gui.md` §1): only four modules touch the browser globals, the
 * state never imports a view, and the import graph has no cycle. Mirrors the extension's `tests/layout.test.ts`.
 */
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";

const SRC = join(import.meta.dir, "..", "src");

/** The modules that may touch the browser: the entry point, the hash source, the form field and the DOM patcher. */
const BROWSER_MODULES = ["main.ts", "router/hash-source.ts", "view/form-field.ts", "view/mount.ts"];

/** A browser global used as a name of its own, not as a property such as `state.history`. */
const BROWSER_GLOBAL =
	/(?<![\w$.])(document|window|location|history|navigator|localStorage|sessionStorage|fetch|FormData|HTML\w*Element)\b/;

/** An import edge between two modules, as paths relative to `src/` with `/` separators. */
interface Edge {
	from: string;
	to: string;
}

/** Every `.ts` file below `dir`, absolute. */
function tsFiles(dir: string): string[] {
	return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
		const path = join(dir, entry.name);
		if (entry.isDirectory()) return tsFiles(path);
		return entry.name.endsWith(".ts") ? [path] : [];
	});
}

/** A path relative to `src/` with `/` separators. */
function relOf(path: string): string {
	return relative(SRC, path).split(sep).join("/");
}

/** The source of a module without its comments (one leftmost pass, so `/*` inside a line comment hides nothing). */
function code(rel: string): string {
	return readFileSync(join(SRC, rel), "utf8").replace(/\/\*[\s\S]*?\*\/|(^|[^:])\/\/.*$/gm, "$1");
}

/** The modules a module imports, resolved against its folder. */
function importsOf(rel: string): string[] {
	return [...code(rel).matchAll(/(?:from|import)\s*\(?\s*["'](\.[^"']+)["']/g)].map((match) =>
		relOf(resolve(dirname(join(SRC, rel)), match[1] ?? "")),
	);
}

const MODULES = tsFiles(SRC).map(relOf);
const EDGES: Edge[] = MODULES.flatMap((from) => importsOf(from).map((to) => ({ from, to })));

/** The modules that sit on an import cycle. */
function modulesInCycles(): string[] {
	const state = new Map<string, "open" | "closed">();
	const inCycle = new Set<string>();
	const visit = (module: string, path: string[]): void => {
		state.set(module, "open");
		for (const { to } of EDGES.filter((edge) => edge.from === module)) {
			if (state.get(to) === "open") {
				for (const member of path.slice(path.indexOf(to))) inCycle.add(member);
			} else if (!state.has(to)) {
				visit(to, [...path, to]);
			}
		}
		state.set(module, "closed");
	};
	for (const module of MODULES) if (!state.has(module)) visit(module, [module]);
	return [...inCycle].sort();
}

describe("gui layout", () => {
	test("the browser modules exist", () => {
		expect(BROWSER_MODULES.filter((module) => !MODULES.includes(module))).toEqual([]);
	});

	test("only the browser modules touch the browser globals", () => {
		const offenders = MODULES.filter((module) => !BROWSER_MODULES.includes(module) && BROWSER_GLOBAL.test(code(module)));
		expect(offenders).toEqual([]);
	});

	test("the state never imports a view", () => {
		const offenders = EDGES.filter((edge) => edge.from.startsWith("state/") && edge.to.startsWith("views/"));
		expect(offenders.map((edge) => `${edge.from} -> ${edge.to}`)).toEqual([]);
	});

	test("every relative import resolves to a module", () => {
		const unresolved = EDGES.filter((edge) => !MODULES.includes(edge.to));
		expect(unresolved.map((edge) => `${edge.from} -> ${edge.to}`)).toEqual([]);
	});

	test("the import graph has no cycle", () => {
		expect(modulesInCycles()).toEqual([]);
	});
});
