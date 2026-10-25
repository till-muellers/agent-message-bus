import type { LaunchContext, Launcher } from "./launcher.ts";

/** The first launcher, in order, whose environment is detected. */
export async function pickLauncher(launchers: readonly Launcher[], ctx: LaunchContext): Promise<Launcher | undefined> {
	for (const launcher of launchers) {
		if (await launcher.detect(ctx)) return launcher;
	}
	return undefined;
}
