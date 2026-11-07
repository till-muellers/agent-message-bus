"""Tests for scripts/release_ci_gate.py: the release publishes only for a commit whose full-tier run passed."""

from __future__ import annotations

import importlib.util
import io
import json
import sys
from pathlib import Path
from types import ModuleType

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "release_ci_gate.py"
SHA = "03845f2a"


def load_module() -> ModuleType:
    spec = importlib.util.spec_from_file_location("release_ci_gate", SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules["release_ci_gate"] = module
    spec.loader.exec_module(module)
    return module


release_ci_gate = load_module()


def run(status: str, conclusion: str | None, branch: str = "main", **fields: object) -> dict[str, object]:
    """Return one workflow run of SHA as the GitHub API lists it; `fields` override the defaults."""
    base: dict[str, object] = {"id": 1, "event": "push", "head_sha": SHA, "head_branch": branch}
    return base | {"status": status, "conclusion": conclusion} | fields


def test_a_successful_run_passes() -> None:
    assert release_ci_gate.check(SHA, [run("completed", "success")]) is None


def test_a_successful_run_passes_while_the_tag_run_is_still_running() -> None:
    runs = [run("in_progress", None, "v0.3.0"), run("completed", "success")]
    assert release_ci_gate.check(SHA, runs) is None


def test_a_successful_tag_run_passes() -> None:
    # The tag run of the full tier runs the suites itself when no run of main covered the commit.
    assert release_ci_gate.check(SHA, [run("completed", "success", "v0.3.0")]) is None


def test_a_successful_run_of_another_commit_fails() -> None:
    error = release_ci_gate.check(SHA, [run("completed", "success", head_sha="ffffffff")])
    assert error is not None
    assert "no full-tier run" in error


def test_a_successful_pull_request_run_fails() -> None:
    # A pull_request run tests the merge with the base branch, not the tagged tree.
    error = release_ci_gate.check(SHA, [run("completed", "success", event="pull_request")])
    assert error is not None
    assert "no full-tier run" in error


def test_no_run_fails_and_names_the_commit() -> None:
    error = release_ci_gate.check(SHA, [])
    assert error is not None
    assert SHA in error
    assert "no full-tier run" in error


def test_a_failed_run_fails_and_lists_it() -> None:
    error = release_ci_gate.check(SHA, [run("completed", "failure")])
    assert error is not None
    assert "main: completed/failure" in error


def test_a_run_still_in_progress_fails_and_says_to_rerun() -> None:
    error = release_ci_gate.check(SHA, [run("in_progress", None)])
    assert error is not None
    assert "main: in_progress" in error
    assert "re-run" in error


def test_the_message_points_at_the_full_tier_on_main() -> None:
    error = release_ci_gate.check(SHA, [])
    assert error is not None
    assert "main" in error


def test_parse_runs_reads_the_api_response() -> None:
    body = json.dumps({"total_count": 1, "workflow_runs": [run("completed", "success")]})
    assert release_ci_gate.parse_runs(body) == [run("completed", "success")]


def test_parse_runs_rejects_a_response_without_runs() -> None:
    with pytest.raises(SystemExit) as failure:
        release_ci_gate.parse_runs('{"message": "Not Found"}')
    assert "workflow_runs" in str(failure.value)


def test_main_passes_for_a_green_commit(monkeypatch: pytest.MonkeyPatch) -> None:
    body = json.dumps({"workflow_runs": [run("completed", "success")]})
    monkeypatch.setattr(sys, "stdin", io.StringIO(body))
    assert release_ci_gate.main(["--sha", SHA]) == 0


def test_main_fails_for_a_commit_without_a_full_tier_run(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(sys, "stdin", io.StringIO('{"workflow_runs": []}'))
    assert release_ci_gate.main(["--sha", SHA]) == 1
