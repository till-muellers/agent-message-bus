/**
 * The leader's omp flags that load extensions, forwarded to a spawned worker: omp rejects unknown flags, and
 * `--bus-url`/`--bus-accept` exist only while this extension is loaded. A launcher may start the worker in another
 * directory, so relative `--plugin-dir` and `-e`/`--extension` values (bare names included: omp resolves them as paths)
 * are resolved against the leader's cwd; `~` values, absolute values and `--trusted-extension` (omp requires it absolute)
 * are forwarded unchanged.
 */
import { isAbsolute, resolve } from "node:path";

/** Flags whose relative values are resolved against the leader's cwd. */
const RESOLVED_FLAGS: readonly string[] = ["--plugin-dir", "--extension", "-e"];
const FLAGS: readonly string[] = [...RESOLVED_FLAGS, "--trusted-extension"];

function forwardedValue(flag: string, value: string, cwd: string): string {
	const resolvable = RESOLVED_FLAGS.includes(flag) && !value.startsWith("~") && !isAbsolute(value);
	return resolvable ? resolve(cwd, value) : value;
}

/** `--flag=<p>` forms of the forwarded long flags; `undefined` for any other argument. */
function equalsForm(arg: string): [flag: string, value: string] | undefined {
	const eq = arg.indexOf("=");
	const flag = arg.slice(0, eq);
	return eq > 0 && flag.startsWith("--") && FLAGS.includes(flag) ? [flag, arg.slice(eq + 1)] : undefined;
}

/** The extension-loading flags in `argv` (both forms), in order, up to `--`; relative paths resolved against `cwd`. */
export function pluginArgs(argv: readonly string[], cwd: string = process.cwd()): string[] {
	const out: string[] = [];
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === "--") break;
		const equals = equalsForm(arg);
		if (equals) out.push(`${equals[0]}=${forwardedValue(equals[0], equals[1], cwd)}`);
		else if (FLAGS.includes(arg) && i + 1 < argv.length) out.push(arg, forwardedValue(arg, argv[++i], cwd));
	}
	return out;
}
