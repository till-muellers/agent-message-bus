"""Tests for the pure helpers in scripts/release_version.py."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "release_version.py"


def load_module() -> ModuleType:
    spec = importlib.util.spec_from_file_location("release_version", SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules["release_version"] = module
    spec.loader.exec_module(module)
    return module


release_version = load_module()


def write_package(tmp_path: Path, version: str) -> Path:
    """Write a package.json with the given version and return its path."""
    path = tmp_path / "package.json"
    path.write_text('{"name": "x", "version": "%s"}\n' % version, encoding="utf-8")
    return path


def test_package_version_reads_the_version(tmp_path: Path) -> None:
    assert release_version.package_version(write_package(tmp_path, "0.2.0")) == "0.2.0"


def test_package_version_without_a_version_is_an_error(tmp_path: Path) -> None:
    path = tmp_path / "package.json"
    path.write_text('{"name": "x"}\n', encoding="utf-8")
    with pytest.raises(SystemExit) as failure:
        release_version.package_version(path)
    assert "version" in str(failure.value)


def test_a_matching_tag_passes() -> None:
    assert release_version.check("v0.2.0", "0.2.0") is None


def test_a_mismatched_tag_names_both_versions() -> None:
    assert (
        release_version.check("v0.2.0", "0.1.0")
        == "package version 0.1.0 does not match tag v0.2.0"
    )


def test_the_v_prefix_is_optional() -> None:
    assert release_version.check("0.2.0", "0.2.0") is None
    assert release_version.check("v0.2.0", "0.2.0") is None
