#!/usr/bin/env python3
"""
Check that a release tag matches the version of the plugin's package.json.

The plugin version lives in `package.json` and is bumped in every release commit; the tag marks the
release. The release workflow runs this check first, so a mismatched tag fails in seconds and
publishes nothing (`docs/design/arch/pipeline.md` §4).

Usage: python scripts/release_version.py --tag vX.Y.Z
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
PACKAGE_JSON = REPO_ROOT / "package.json"


def package_version(package_json: Path) -> str:
    """Return the version of the given package.json, or exit with a clear message."""
    try:
        parsed = json.loads(package_json.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        sys.exit(f"error: cannot read {package_json}: {error}")
    version = parsed.get("version") if isinstance(parsed, dict) else None
    if not isinstance(version, str) or not version:
        sys.exit(f"error: {package_json} has no version")
    return version


def check(tag: str, version: str) -> str | None:
    """Return the error message when the tag does not match the version, else None."""
    if tag.removeprefix("v") == version:
        return None
    return f"package version {version} does not match tag {tag}"


def parse_args(argv: list[str] | None) -> argparse.Namespace:
    """Parse the command line."""
    parser = argparse.ArgumentParser(description="Check that a release tag matches the plugin version.")
    parser.add_argument("--tag", required=True, help="the release tag, for example v0.2.0")
    parser.add_argument("--package", default=str(PACKAGE_JSON), help="path to package.json")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    """Check the tag against the package version; return the exit code."""
    args = parse_args(argv)
    version = package_version(Path(args.package))
    error = check(args.tag, version)
    if error is None:
        print(f"tag {args.tag} matches the package version {version}")
        return 0
    print(f"error: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
