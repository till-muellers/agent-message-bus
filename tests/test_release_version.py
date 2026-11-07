"""Tests for the pure helpers in scripts/release_version.py."""

from __future__ import annotations

import importlib.util
import json
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
    path.write_text(json.dumps({"name": "x", "version": version}) + "\n", encoding="utf-8")
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
    assert release_version.check("v0.2.0", "0.1.0") == "package version 0.1.0 does not match tag v0.2.0"


@pytest.mark.parametrize(
    "version",
    [
        "0.0.0",
        "0.4.0",
        "1.2.3",
        "10.20.30",
        "1.2.3-rc.1",
        "3.5.1-prerelease01",
        "1.2.3-0",
        "1.2.3-alpha.beta-1",
        "1.0.0-x.7.z.92",
    ],
)
def test_a_semver_tag_equal_to_the_version_passes(version: str) -> None:
    assert release_version.check(f"v{version}", version) is None


@pytest.mark.parametrize(
    "tag",
    [
        "v1.2",
        "v1",
        "v01.2.3",
        "v1.02.3",
        "v1.2.03",
        "v1.2.3-01",
        "v1.2.3-",
        "v1.2.3-rc..1",
        "v1.2.3+build",
        "v1.2.3-rc.1+build.5",
        "1.2.3",
        "V1.2.3",
        "v1.2.3.4",
        "v1.2.x",
    ],
)
def test_a_tag_that_is_not_a_semver_version_fails(tag: str) -> None:
    error = release_version.check(tag, tag.removeprefix("v"))
    assert error is not None
    assert tag in error
    assert "vMAJOR.MINOR.PATCH" in error


def test_build_metadata_is_refused_and_the_message_says_why() -> None:
    error = release_version.check("v1.2.3+5", "1.2.3+5")
    assert error is not None
    assert "build metadata" in error


def test_the_v_prefix_is_required() -> None:
    assert release_version.check("0.2.0", "0.2.0") is not None


def test_a_semver_tag_is_checked_before_the_version_comparison() -> None:
    error = release_version.check("v1.2", "0.1.0")
    assert error is not None
    assert "does not match" not in error


def test_a_pre_release_tag_must_equal_the_package_version_exactly() -> None:
    assert release_version.check("v1.2.3-rc.1", "1.2.3") == "package version 1.2.3 does not match tag v1.2.3-rc.1"
    assert release_version.check("v1.2.3", "1.2.3-rc.1") == "package version 1.2.3-rc.1 does not match tag v1.2.3"
