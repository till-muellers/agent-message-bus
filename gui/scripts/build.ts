import { mkdirSync, rmSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { parseArgs } from "node:util";

const GUI_DIR = resolve(import.meta.dir, "..");
const DEFAULT_OUT = resolve(GUI_DIR, "..", "server", "internal", "web", "dist");
const ASSET_URL = "/web/assets/";

/** The bundle files index.html refers to, by their template placeholder. */
type Bundle = Readonly<Record<"%MAIN_SCRIPT%" | "%STYLE%", string>>;

async function bundleAssets(assetsDir: string): Promise<Bundle> {
	const result = await Bun.build({
		entrypoints: [join(GUI_DIR, "src", "main.ts"), join(GUI_DIR, "src", "style.css")],
		outdir: assetsDir,
		target: "browser",
		minify: true,
		naming: "[name]-[hash].[ext]",
		sourcemap: "none",
	});
	const script = result.outputs.find((output) => output.path.endsWith(".js"));
	const style = result.outputs.find((output) => output.path.endsWith(".css"));
	if (!result.success || !script || !style) {
		throw new AggregateError(result.logs, "GUI bundle failed");
	}
	return { "%MAIN_SCRIPT%": basename(script.path), "%STYLE%": basename(style.path) };
}

async function writeIndex(outDir: string, bundle: Bundle): Promise<void> {
	let html = await Bun.file(join(GUI_DIR, "index.html")).text();
	for (const [placeholder, file] of Object.entries(bundle)) {
		if (!html.includes(placeholder)) throw new Error(`index.html lacks the ${placeholder} placeholder`);
		html = html.replace(placeholder, ASSET_URL + file);
	}
	await Bun.write(join(outDir, "index.html"), html);
}

/** Builds the GUI into outDir: outDir/assets/main-<hash>.js, style-<hash>.css and outDir/index.html. */
export async function buildGui(outDir: string): Promise<void> {
	const assetsDir = join(outDir, "assets");
	rmSync(assetsDir, { recursive: true, force: true });
	mkdirSync(assetsDir, { recursive: true });
	await writeIndex(outDir, await bundleAssets(assetsDir));
}

if (import.meta.main) {
	const { values } = parseArgs({ options: { out: { type: "string" } } });
	const outDir = resolve(values.out ?? DEFAULT_OUT);
	await buildGui(outDir);
	console.log(`GUI built into ${outDir}`);
}
