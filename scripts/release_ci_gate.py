#!/usr/bin/env python3
"""
Check that the `ci` workflow passed for the commit a release is built from.

The release workflow runs this check first, so a tag on a commit whose ci run failed, is still
running or never ran fails in seconds and publishes nothing (`docs/design/arch/pipeline.md` §4).
It reads the GitHub API response that lists the ci runs of the commit from stdin:

    gh api "repos/OWNER/REPO/actions/workflows/ci.yml/runs?head_sha=SHA&event=push&per_page=100" \
        | python scripts/release_ci_gate.py --sha SHA

Only `push` runs of the commit itself count: a `pull_request` run tests the merge with the base
branch, not the tagged tree. One successful run is enough: the tag push starts its own ci run of
the same commit, which is still running while the release checks the run of the branch push.
"""

from __future__ import annotations

import argparse
import json
import sys

Run = dict[str, object]


def parse_runs(body: str) -> list[Run]:
    """Return the workflow runs of the API response, or exit with a clear message."""
    try:
        parsed = json.loads(body)
    except json.JSONDecodeError as error:
        sys.exit(f"error: the ci runs response is not JSON: {error}")
    runs = parsed.get("workflow_runs") if isinstance(parsed, dict) else None
    if not isinstance(runs, list):
        sys.exit(f"error: the ci runs response has no workflow_runs: {body[:200]}")
    return runs


def describe(run: Run) -> str:
    """Return one run as `branch: status[/conclusion]`."""
    state = run.get("status")
    if run.get("conclusion"):
        state = f"{state}/{run['conclusion']}"
    return f"{run.get('head_branch')}: {state}"


def check(sha: str, runs: list[Run]) -> str | None:
    """Return the error message when no push ci run of the commit succeeded, else None."""
    runs = [run for run in runs if run.get("head_sha") == sha and run.get("event") == "push"]
    if any(run.get("conclusion") == "success" for run in runs):
        return None
    if not runs:
        return f"no ci run for commit {sha}; tag a commit whose ci run passed"
    listed = ", ".join(describe(run) for run in runs)
    return f"no successful ci run for commit {sha} ({listed}); re-run this release once ci passed"


def parse_args(argv: list[str] | None) -> argparse.Namespace:
    """Parse the command line."""
    parser = argparse.ArgumentParser(description="Check that the ci workflow passed for the release commit.")
    parser.add_argument("--sha", required=True, help="the commit the release is built from")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    """Check the ci runs read from stdin; return the exit code."""
    args = parse_args(argv)
    error = check(args.sha, parse_runs(sys.stdin.read()))
    if error is None:
        print(f"ci passed for commit {args.sha}")
        return 0
    print(f"error: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
