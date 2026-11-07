#!/usr/bin/env python3
"""
Check that the full-tier workflow (`full.yml`) passed for the commit a release is built from.

The release workflow runs this check first, so a tag on a commit whose full-tier run failed, is still
running or never ran fails in seconds and publishes nothing (`docs/design/arch/pipeline.md` §4).
It reads the GitHub API response that lists the full-tier runs of the commit from stdin:

    gh api "repos/OWNER/REPO/actions/workflows/full.yml/runs?head_sha=SHA&event=push&per_page=100" \
        | python scripts/release_ci_gate.py --sha SHA

Only `push` runs of the commit itself count: a `pull_request` run tests the merge with the base
branch, not the tagged tree (the full tier has no such trigger; the filter keeps a broken query from
passing). One successful run is enough: it is the run of the merge to `main`, or the run of the tag
push when no run of `main` covered the commit; the tag push also starts its own full-tier run, which
may still be in progress while the release checks.
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
        sys.exit(f"error: the full-tier runs response is not JSON: {error}")
    runs = parsed.get("workflow_runs") if isinstance(parsed, dict) else None
    if not isinstance(runs, list):
        sys.exit(f"error: the full-tier runs response has no workflow_runs: {body[:200]}")
    return runs


def describe(run: Run) -> str:
    """Return one run as `branch: status[/conclusion]`."""
    state = run.get("status")
    if run.get("conclusion"):
        state = f"{state}/{run['conclusion']}"
    return f"{run.get('head_branch')}: {state}"


def check(sha: str, runs: list[Run]) -> str | None:
    """Return the error message when no push run of the full tier succeeded for the commit, else None."""
    runs = [run for run in runs if run.get("head_sha") == sha and run.get("event") == "push"]
    if any(run.get("conclusion") == "success" for run in runs):
        return None
    if not runs:
        return f"no full-tier run for commit {sha}; merge to main, wait until the full tier passed there, then tag it"
    listed = ", ".join(describe(run) for run in runs)
    return f"no successful full-tier run for commit {sha} ({listed}); re-run this release once the full tier passed"


def parse_args(argv: list[str] | None) -> argparse.Namespace:
    """Parse the command line."""
    parser = argparse.ArgumentParser(description="Check that the full-tier workflow passed for the release commit.")
    parser.add_argument("--sha", required=True, help="the commit the release is built from")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    """Check the full-tier runs read from stdin; return the exit code."""
    args = parse_args(argv)
    error = check(args.sha, parse_runs(sys.stdin.read()))
    if error is None:
        print(f"full tier passed for commit {args.sha}")
        return 0
    print(f"error: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
