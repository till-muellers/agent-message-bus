import { mkdirSync, rmSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { parseArgs } from "node:util";

const GUI_DIR = resolve(import.meta.dir, "..");
const DEFAULT_OUT = resolve(GUI_DIR, "..", "server", "internal", "web", "dist");
const SCRIPT_PLACEHOLDER = "%MAIN_SCRIPT%";
const ASSET_URL = "/web/assets/";

async function bundleMain(assetsDir: string): Promise<string> {
	const result = await Bun.build({
		entrypoints: [join(GUI_DIR, "src", "main.ts")],
		outdir: assetsDir,
		target: "browser",
		minify: true,
		naming: "[name]-[hash].[ext]",
		sourcemap: "none",
	});
	const entry = result.outputs.find((output) => output.kind === "entry-point");
	if (!result.success || !entry) {
		throw new AggregateError(result.logs, "GUI bundle failed");
	}
	return basename(entry.path);
}

async function writeIndex(outDir: string, bundle: string): Promise<void> {
	const template = await Bun.file(join(GUI_DIR, "index.html")).text();
	if (!template.includes(SCRIPT_PLACEHOLDER)) {
		throw new Error(`index.html lacks the ${SCRIPT_PLACEHOLDER} placeholder`);
	}
	await Bun.write(join(outDir, "index.html"), template.replace(SCRIPT_PLACEHOLDER, ASSET_URL + bundle));
}

/** Builds the GUI into outDir: outDir/assets/main-<hash>.js and outDir/index.html. */
export async function buildGui(outDir: string): Promise<void> {
	const assetsDir = join(outDir, "assets");
	rmSync(assetsDir, { recursive: true, force: true });
	mkdirSync(assetsDir, { recursive: true });
	await writeIndex(outDir, await bundleMain(assetsDir));
}

if (import.meta.main) {
	const { values } = parseArgs({ options: { out: { type: "string", default: DEFAULT_OUT } } });
	const outDir = resolve(values.out ?? DEFAULT_OUT);
	await buildGui(outDir);
	console.log(`GUI built into ${outDir}`);
}
