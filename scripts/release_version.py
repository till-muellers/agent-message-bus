#!/usr/bin/env python3
"""
Check that a release tag is a Semantic Versioning 2.0.0 version and matches the version of the plugin's
package.json.

The plugin version lives in `package.json`; the tag marks the release and is `v` plus that exact version, so
a release candidate commit carries `0.5.0-rc.1` in `package.json` and is tagged `v0.5.0-rc.1`. A `v*` tag
publishes a release, so only `vMAJOR.MINOR.PATCH[-PRERELEASE]` is accepted: no leading zeros, no empty or
zero-padded numeric pre-release identifier and no build metadata (`+5`, ignored by precedence and awkward in
asset names). The release workflow runs this check first, before the full-tier gate (`release_ci_gate.py`),
so a mismatched or malformed tag fails in seconds and publishes nothing (`docs/design/arch/pipeline.md` §4).

Usage: python scripts/release_version.py --tag vMAJOR.MINOR.PATCH[-PRERELEASE]
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
PACKAGE_JSON = REPO_ROOT / "package.json"
# SemVer 2.0.0 (semver.org) `<version core>[-<pre-release>]`; build metadata (`+...`) is left out on purpose.
SEMVER_TAG = re.compile(
    r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*)?"
)


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
    """Return the error message when the tag is no SemVer core tag or does not match the version, else None."""
    if SEMVER_TAG.fullmatch(tag) is None:
        return (
            f"tag {tag} is not of the form vMAJOR.MINOR.PATCH[-PRERELEASE] (Semantic Versioning 2.0.0: "
            "no leading zeros, no empty or zero-padded numeric pre-release identifier; "
            "build metadata (+...) is not accepted: precedence ignores it and it is awkward in asset names)"
        )
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
