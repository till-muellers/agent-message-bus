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


def test_targets_cover_windows_and_linux_only() -> None:
    assert build.targets() == [
        ("windows", "amd64"),
        ("windows", "arm64"),
        ("linux", "amd64"),
        ("linux", "arm64"),
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
        ("dev", "linux", "arm64", "agent-message-bus_dev_linux_arm64"),
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


def test_gui_build_commands_install_then_build_into_the_out_dir(tmp_path: Path) -> None:
    commands, cwd = build.gui_build_commands("bun", tmp_path)
    assert commands == [
        ["bun", "install", "--frozen-lockfile"],
        ["bun", "run", "build", "--out", str(tmp_path)],
    ]
    assert cwd == build.REPO_ROOT / "gui"


def test_gui_dist_is_the_embed_dir_of_the_server() -> None:
    assert build.GUI_DIST == build.APP_DIR / "internal" / "web" / "dist"


@pytest.mark.parametrize(("argv", "skip"), [([], False), (["--skip-gui"], True)])
def test_skip_gui_flag(argv: list[str], skip: bool) -> None:
    assert build.parse_args(argv).skip_gui is skip


def test_find_bun_returns_the_bun_on_path(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(build.shutil, "which", lambda name: f"/usr/bin/{name}")
    assert build.find_bun() == "/usr/bin/bun"


def test_find_bun_exits_when_bun_is_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(build.shutil, "which", lambda name: None)
    with pytest.raises(SystemExit, match="--skip-gui"):
        build.find_bun()
