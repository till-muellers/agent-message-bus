"""Tests for the pure helpers in scripts/build.py."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "build.py"


def load_module() -> ModuleType:
    spec = importlib.util.spec_from_file_location("build", SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules["build"] = module
    spec.loader.exec_module(module)
    return module


build = load_module()


def test_targets_cover_all_six_combinations() -> None:
    assert build.targets() == [
        ("windows", "amd64"),
        ("windows", "arm64"),
        ("linux", "amd64"),
        ("linux", "arm64"),
        ("darwin", "amd64"),
        ("darwin", "arm64"),
    ]


def test_app_dir_is_the_go_module() -> None:
    assert build.APP_DIR == build.REPO_ROOT / "server"
    assert (build.APP_DIR / "go.mod").is_file()


@pytest.mark.parametrize(
    ("version", "goos", "goarch", "expected"),
    [
        ("1.2.3", "windows", "amd64", "agent-message-bus_1.2.3_windows_amd64.exe"),
        ("1.2.3", "windows", "arm64", "agent-message-bus_1.2.3_windows_arm64.exe"),
        ("dev", "linux", "amd64", "agent-message-bus_dev_linux_amd64"),
        ("dev", "darwin", "arm64", "agent-message-bus_dev_darwin_arm64"),
    ],
)
def test_artifact_name(version: str, goos: str, goarch: str, expected: str) -> None:
    assert build.artifact_name(version, goos, goarch) == expected


@pytest.mark.parametrize(
    ("digest", "filename", "expected"),
    [
        ("abc123", "file.exe", "abc123  file.exe"),
        ("0" * 64, "agent-message-bus_dev_linux_amd64", "0" * 64 + "  agent-message-bus_dev_linux_amd64"),
    ],
)
def test_checksum_line(digest: str, filename: str, expected: str) -> None:
    assert build.checksum_line(digest, filename) == expected


@pytest.mark.parametrize(
    ("entries", "expected"),
    [
        ({}, ""),
        ({"b": "22", "a": "11"}, "11  a\n22  b\n"),
    ],
)
def test_checksums_text_sorted(entries: dict[str, str], expected: str) -> None:
    assert build.checksums_text(entries) == expected


@pytest.mark.parametrize(
    ("out", "absolute"),
    [("dist", False), (str(Path.cwd().anchor + "abs-out"), True)],
)
def test_resolve_out(out: str, absolute: bool) -> None:
    resolved = build.resolve_out(out)
    expected = Path(out) if absolute else build.REPO_ROOT / out
    assert resolved == expected
