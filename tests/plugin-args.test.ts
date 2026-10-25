import { describe, expect, test } from "bun:test";
import { resolve } from "node:path";
import { pluginArgs } from "../extension/lib/plugin-args.ts";

const CWD = resolve("/leader");
const ABS = resolve("/p");

describe("pluginArgs", () => {
	test("keeps --plugin-dir, --extension, -e and --trusted-extension with their values, and their = forms, in order", () => {
		const argv = ["bun", "/omp/cli.ts", "--plugin-dir", ABS, "--model", "m", "-e", ABS, `--extension=${ABS}`, `--plugin-dir=${ABS}`, "--trusted-extension", ABS, "hello"];
		expect(pluginArgs(argv, CWD)).toEqual(["--plugin-dir", ABS, "-e", ABS, `--extension=${ABS}`, `--plugin-dir=${ABS}`, "--trusted-extension", ABS]);
	});

	test("resolves relative --plugin-dir values against the cwd", () => {
		const argv = ["omp", "--plugin-dir", ".", "--plugin-dir=plugins"];
		expect(pluginArgs(argv, CWD)).toEqual(["--plugin-dir", CWD, `--plugin-dir=${resolve(CWD, "plugins")}`]);
	});

	test("forwards --trusted-extension values unchanged", () => {
		const argv = ["omp", "--trusted-extension", "ext/a.ts", "--trusted-extension=b.ts"];
		expect(pluginArgs(argv, CWD)).toEqual(["--trusted-extension", "ext/a.ts", "--trusted-extension=b.ts"]);
	});

	test("resolves every relative -e/--extension value, bare names included; ~ and absolute values stay as given", () => {
		const argv = ["omp", "-e", "./x.ts", "--extension", "lib/y.ts", "--extension=..\\z.ts", "-e", "index.ts", "-e", "~/ext.ts", "--extension=~/w.ts", "-e", ABS];
		const expected = ["-e", resolve(CWD, "x.ts"), "--extension", resolve(CWD, "lib/y.ts"), `--extension=${resolve(CWD, "..\\z.ts")}`, "-e", resolve(CWD, "index.ts"), "-e", "~/ext.ts", "--extension=~/w.ts", "-e", ABS];
		expect(pluginArgs(argv, CWD)).toEqual(expected);
	});

	test("stops at --, and drops a flag without a value", () => {
		expect(pluginArgs(["omp", "-e", ABS, "--", "--plugin-dir", "/p"], CWD)).toEqual(["-e", ABS]);
		expect(pluginArgs(["omp", "--plugin-dir"], CWD)).toEqual([]);
		expect(pluginArgs(["omp", "say", "hi"], CWD)).toEqual([]);
	});
});
