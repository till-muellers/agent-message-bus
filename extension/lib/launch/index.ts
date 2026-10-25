/**
 * Launcher detection order: multiplexers first, because the innermost surface wins (an Orca terminal still sets
 * `TERM_PROGRAM=Orca` inside tmux or zellij); tmux beats zellij when both are set; `windows-console` is the win32 fallback.
 */
import { kittyLauncher } from "./kitty.ts";
import type { Launcher } from "./launcher.ts";
import { linuxTerminalLauncher } from "./linux-terminal.ts";
import { macosTerminalLauncher } from "./macos-terminal.ts";
import { orcaLauncher } from "./orca.ts";
import { tmuxLauncher } from "./tmux.ts";
import { weztermLauncher } from "./wezterm.ts";
import { windowsConsoleLauncher } from "./windows-console.ts";
import { windowsTerminalLauncher } from "./windows-terminal.ts";
import { zellijLauncher } from "./zellij.ts";

export const LAUNCHERS: readonly Launcher[] = [
	tmuxLauncher,
	zellijLauncher,
	orcaLauncher,
	weztermLauncher,
	kittyLauncher,
	windowsTerminalLauncher,
	macosTerminalLauncher,
	linuxTerminalLauncher,
	windowsConsoleLauncher,
];
