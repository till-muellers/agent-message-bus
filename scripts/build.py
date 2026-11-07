#!/usr/bin/env python3
"""
Build the web GUI into the server's embed dir, then cross-compile the agent-message-bus server in
`server/` into raw release binaries plus SHA256SUMS.

Writes `agent-message-bus_<version>_<os>_<arch>[.exe]` for every target and a `SHA256SUMS` file into the
output directory; the release workflow uploads exactly these files for a `v<version>` tag. The GUI step
needs bun; `--skip-gui` builds Go only and embeds whatever `server/internal/web/dist/` holds (on a fresh
checkout: nothing, so `/web/` serves the fallback page).

Usage: python scripts/build.py [--version X] [--out dist] [--skip-gui]
"""

from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
APP_DIR = REPO_ROOT / "server"
GUI_DIR = REPO_ROOT / "gui"
GUI_DIST = APP_DIR / "internal" / "web" / "dist"
BINARY_NAME = "agent-message-bus"
OPERATING_SYSTEMS = ("windows", "linux")
ARCHITECTURES = ("amd64", "arm64")
WINDOWS_GO = Path("C:/Program Files/Go/bin/go.exe")


def targets() -> list[tuple[str, str]]:
    """Return every (GOOS, GOARCH) pair to build."""
    return [(goos, goarch) for goos in OPERATING_SYSTEMS for goarch in ARCHITECTURES]


def artifact_name(version: str, goos: str, goarch: str) -> str:
    """Return the release file name for one target."""
    suffix = ".exe" if goos == "windows" else ""
    return f"{BINARY_NAME}_{version}_{goos}_{goarch}{suffix}"


def checksum_line(hex_digest: str, filename: str) -> str:
    """Format one line in `sha256sum` output format."""
    return f"{hex_digest}  {filename}"


def checksums_text(entries: dict[str, str]) -> str:
    """Render a SHA256SUMS body from {filename: hex digest}, sorted by filename."""
    return "".join(checksum_line(entries[name], name) + "\n" for name in sorted(entries))


def find_go() -> str:
    """Locate the go executable or exit with a clear message."""
    found = shutil.which("go")
    if found:
        return found
    if os.name == "nt" and WINDOWS_GO.is_file():
        return str(WINDOWS_GO)
    sys.exit(
        "error: 'go' not found on PATH"
        + (f" or at {WINDOWS_GO}" if os.name == "nt" else "")
        + "; install Go (https://go.dev/dl/) and retry."
    )


def find_bun() -> str:
    """Locate the bun executable or exit with a clear message."""
    found = shutil.which("bun")
    if found:
        return found
    sys.exit("error: 'bun' not found on PATH; install bun (https://bun.sh) or pass --skip-gui for a Go-only build.")


def gui_build_commands(bun: str, out_dir: Path) -> tuple[list[list[str]], Path]:
    """Return the commands that build the GUI into out_dir, and the directory to run them in."""
    return [
        [bun, "install", "--frozen-lockfile"],
        [bun, "run", "build", "--out", str(out_dir)],
    ], GUI_DIR


def build_gui(bun: str, out_dir: Path) -> None:
    """Build the GUI into out_dir, the embed dir of the server."""
    commands, cwd = gui_build_commands(bun, out_dir)
    for cmd in commands:
        print(f"running {' '.join(cmd[1:])} in {cwd.name}/")
        result = subprocess.run(cmd, cwd=cwd, check=False)
        if result.returncode != 0:
            sys.exit(f"error: GUI build step '{' '.join(cmd[1:])}' failed (exit {result.returncode})")


def build_target(go: str, out_file: Path, goos: str, goarch: str) -> None:
    """Build one static binary for goos/goarch into out_file."""
    env = {**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"}
    cmd = [go, "build", "-trimpath", "-ldflags=-s -w", "-o", str(out_file), "."]
    print(f"building {out_file.name}")
    result = subprocess.run(cmd, cwd=APP_DIR, env=env, check=False)
    if result.returncode != 0:
        sys.exit(f"error: go build failed for {goos}/{goarch} (exit {result.returncode})")


def sha256_file(path: Path) -> str:
    """Return the hex SHA-256 digest of a file."""
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def resolve_out(out: str) -> Path:
    """Resolve --out relative to the repo root unless absolute."""
    path = Path(out)
    return path if path.is_absolute() else REPO_ROOT / path


def parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build the GUI, then cross-compile the release binaries.")
    parser.add_argument("--version", default="dev", help="version embedded in artifact names (default: dev)")
    parser.add_argument("--out", default="dist", help="output directory, relative to the repo root (default: dist)")
    parser.add_argument("--skip-gui", action="store_true", help="build Go only, embedding the current GUI dist dir")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if not args.skip_gui:
        build_gui(find_bun(), GUI_DIST)
    go = find_go()
    out_dir = resolve_out(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)
    sums: dict[str, str] = {}
    for goos, goarch in targets():
        out_file = out_dir / artifact_name(args.version, goos, goarch)
        build_target(go, out_file, goos, goarch)
        sums[out_file.name] = sha256_file(out_file)
    (out_dir / "SHA256SUMS").write_text(checksums_text(sums), encoding="utf-8", newline="\n")
    print(f"wrote {len(sums)} binaries and SHA256SUMS to {out_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
