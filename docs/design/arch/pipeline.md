# Pipeline architecture: CI, release, build, Docker, root package.json

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`.

Owner: Arch-client. Files: `.github/workflows/ci-go.yml`, `ci-typescript.yml`, `ci-web.yml`, `ci-python.yml` (cheap tier, §2), `full.yml` (full tier, §2), `release.yml` (publisher, §4), `scripts/build.py` (GUI build, then Go cross-compile only for the targets), `server/Dockerfile` (context `server/`), root `package.json` (`version`, `omp.extensions`, `files`). The branch model they serve is Git Flow (`.omp/AGENTS.md` "Git Flow and CI").
Distribution: **GitHub only, no npm** (findings item 8, PO round 2). PO decisions: PO-10 (GUI build before `go build`, committed placeholder), PO-11 (S1 spawn-time version check). `CA/` = `node_modules/@oh-my-pi/pi-coding-agent/src/` (omp 18.12.6). Server-side contracts: `docs/design/arch/contracts.md` §6 (protocol), §10.5 (static assets).

---

## 1. GUI build before `go build`

The Go binary embeds `server/internal/web/dist/` (Arch-server owns `embed.go`, `dist/.gitkeep`, `server/internal/web/.gitignore` and the fallback page, contracts.md §10.5). Every place that produces a **release** binary must run the GUI build first; every place that only **tests** Go works without it (the fallback page is served).

| Place | Change |
|---|---|
| `scripts/build.py` | New step before the target loop: `build_gui(out_dir=APP_DIR / "internal/web/dist")` runs `bun install --frozen-lockfile` and `bun run build` in `gui/`. `find_bun()` mirrors `find_go()` (:50-61). Flag `--skip-gui` for Go-only builds (local cross-compile without bun); without the flag a missing bun is an error, so a release can never ship the fallback page silently. The GUI build runs once, not per target. |
| `.github/workflows/release.yml`, `full.yml` (`package`) | `oven-sh/setup-bun@v2` before "Build release binaries"; build.py then builds the GUI. |
| `server/Dockerfile` | **Variant A (recommended, Q-P1, PL-2): unchanged.** The context stays `server/`; the GUI is built on the host first (`bun run build:gui`), and the existing `COPY . .` carries `internal/web/dist` into the image (`server/.dockerignore` excludes only `.git`, `*.db`, `data/`). Without the GUI step the image serves the fallback page. Docs: `server/DEPLOYMENT.md:146`, `server/README.md:39`. No image is published by CI (`release.yml` builds binaries only), so no workflow changes. **Variant B (only with Q-P1 approval):** new first stage `FROM oven/bun:1 AS gui`: `COPY gui/ /src/gui/`, `bun install --frozen-lockfile`, `bun run build --out /out/dist`; Go stage: `COPY server/ .` and `COPY --from=gui /out/dist internal/web/dist`; build context = repo root (`docker build -f server/Dockerfile -t agent-message-bus .`); root `.dockerignore` (`node_modules`, `tmp`, `.git`, `dist`, `.beads`, `.dolt`). `oven/bun` is a new base image and needs approval under "no new dependency without asking". |
| `.github/workflows/ci-web.yml`, `full.yml` | The `web` job (§2). The `go` jobs stay bun-free and test against the fallback page. |
| Local dev | `go build` / `go test` need nothing new. GUI work: `bun run --cwd gui build`; the output is ignored by `server/internal/web/.gitignore`. |
| `.gitignore` | `dist/` → `/dist/` (keeps ignoring the release output at the repo root, stops hiding `server/internal/web/dist/.gitkeep`). Lands in server slice W1a with the `.gitkeep` (requested from Arch-server). |

`build.py` functions stay single-purpose and typed (`.omp/RULES.md`); pytest in `tests/test_build.py` covers `gui_build_commands()` (pure: returns the two argv lists and cwd), `--skip-gui` parsing and `find_bun()` with a patched `shutil.which`.

---

## 2. CI: two tiers, the local checks are the gate

CI costs money (billed per minute). Every workflow runs on `ubuntu-latest` only, never on Windows or macOS (a Windows minute costs about twice and a macOS minute about ten times a Linux minute): we develop on Windows, so Windows is covered by the local runs; CI is the Linux check, and a Linux-only failure can be reproduced locally in WSL (§2 "Linux checks locally"); macOS is covered by local runs of whoever has a Mac. So the regular gate is the local checks (`.omp/AGENTS.md` "Local checks"); CI does not repeat them, and agents never trigger, poll or wait for a run. CI is a safety net on pushed branches (cheap tier) and the full verification of a released state (full tier).

| Workflow | Trigger | Paths (jobs run only when one changed) | Jobs | OS | Concurrency |
|---|---|---|---|---|---|
| `ci-go.yml` | push to `develop`, `feature/**`, `release/**`, `hotfix/**`; `pull_request` | `server/**`, itself | gofmt, `go build`, `go vet`, `go test ./...` (no `-race`), golangci-lint v2.15.0 | ubuntu | `cancel-in-progress` per workflow and branch |
| `ci-typescript.yml` | same | `extension/**`, `tests/**` (not `tests/test_*.py`), `package.json`, `bun.lock`, `bunfig.toml`, `tsconfig.json`, `server/TOOLS.md` and `server/internal/server/protocol.go` (read by the tests), `.github/workflows/**` (read by `tests/workflows.test.ts`) | `bun install --frozen-lockfile`, `bun run typecheck`, `bun test tests/` | ubuntu | same |
| `ci-web.yml` | same | `gui/**`, `server/internal/web/**`, `package.json`, itself | GUI typecheck and tests, `bun run build:gui`, `go build ./...` and `go test ./internal/web/...` against the real bundle | ubuntu | same |
| `ci-python.yml` | same | `scripts/**`, `tests/test_*.py`, `ruff.toml`, itself | `ruff format --check`, `ruff check` (ruff 0.17.3), `pytest tests` | ubuntu | same |
| `full.yml` | push to `main`, push of `v*` tags, `workflow_dispatch`; no `pull_request` | none | `covered`, `go`, `typescript`, `web`, `python`, `package` | ubuntu | `cancel-in-progress` per workflow and ref |
| `release.yml` | push of `v*` tags | none | `release` (publishes) | ubuntu | none |

Rules:
- Pinned tools as before: bun through `bun-version-file: gui/package.json` (Q-P3), ruff `==0.17.3` (the version `ruff.toml` requires, so a local ruff of another version refuses to run instead of formatting differently), Go through `server/go.mod`.
- Path filters are native `paths:` filters, one small workflow per area, so no third-party action and no filter job are needed. The filter counts the workflow's own file. `tests/workflows.test.ts` (typescript area) fails when a cheap workflow loses its `paths:` or `cancel-in-progress`, runs on another OS or on `main`/tags, when `full.yml` gains a `pull_request`, `develop` or `feature` trigger, or when a workflow uses an action other than `actions/*`, `oven-sh/setup-bun` and `golangci/golangci-lint-action`.
- The concurrency group is `<workflow>-<branch>` (`github.head_ref || github.ref_name`), so the push run and the pull-request run of one branch share a group and the older is cancelled.
- A change outside every filter (docs, `.omp/`, `.beads/`) starts no job.
- The `web` job runs the GUI tests and build through the root scripts `test:gui` and `build:gui` (§5), so the commands contributors run locally are the ones CI runs. `go test` in the `go` jobs runs without a GUI bundle (the fallback page); the real-bundle run is the `web` job.

### Full tier

`full.yml` runs on the merge of a `release/*` or `hotfix/*` branch into `main` and on `v*` tags, and adds to the cheap tier:
- the race detector (`go test -race ./...`, needs CGO; the runner has gcc). The Go code has platform files (`detach`, `instance` lock, `server` listen); CI compiles and tests only the Linux ones, so the Windows files are covered by the local runs on the development machine.
- every area on every full run, not path-filtered: TypeScript, GUI, Python, the generated-docs golden test and the `/web` contract fixtures (both inside the Go and GUI suites), the tests that read `server/spec/*.feature`.
- the packaging smoke build (`package`: `scripts/build.py`, the GUI, six binaries and `SHA256SUMS`, checked and discarded; nothing is published).
- `golangci-lint` and `gofmt` once more (the same Linux job).

The `covered` job lets a tag run skip all suites when a successful `full.yml` push run for the tagged commit exists (the run of the merge to `main`), so one commit is not tested twice. If no such run exists (for example a newer push to `main` cancelled it), the tag run executes the suites itself, and `release.yml` accepts that run.

There is no manual-only workflow: no suite in this repo needs a display or a GPU. A browser smoke would go into `manual-*.yml` with `workflow_dispatch` only.

### Linux checks locally

When a Linux-only failure is suspected (or to reproduce a CI failure), run the Go checks in WSL, where `go` and `gcc` are installed: `wsl -e bash -lc "cd /mnt/d/repos/<worktree>/server && go test ./..."` (add `-race` for the full-tier run). WSL keeps its own Go build cache, so the first run compiles everything.

### Cost per typical change (estimate, job runs)

|Step|Before (`ci.yml`)|After|
|---|---|---|
|Push to a feature branch (3 pushes, open PR)|6 runs x 4 jobs (every push and every PR update, no cancel, no filter, `-race`)|3 pushes x 1-2 jobs (only the touched areas, superseded runs cancelled, no `-race`)|
|Land the change|squash merge, 1 run x 4 jobs on `main`|merge to `develop`, 1 run x 1-2 jobs|
|Docs-only change|4 jobs per push|0|
|Release|`main` run (4 jobs), tag run (4 jobs), `go test` again in `release.yml`|`main` run (6 jobs), tag run (1 short job), publish|

Release runs get dearer, feature runs get far cheaper; there are many more features than releases.

## 3. Version and protocol: sources of truth

There are **two** numbers, and they must not be confused.

| Number | Meaning | Single source | Read by |
|---|---|---|---|
| Plugin version (Semantic Versioning 2.0.0 `MAJOR.MINOR.PATCH[-PRERELEASE]`, e.g. `0.2.0`, `0.5.0-rc.1`; precedence in `extension/lib/version/plugin-version.ts` `isOlderVersion`) | which extension code a copy holds; "extension protocol version" in `plugin-version-skew.feature` | root `package.json` `version` (:3) | `omp plugin list` (manifest of the installed commit); `extension/lib/version/plugin-version.ts` at load; release tag check |
| Server protocol (integer, e.g. `2`) | which server contracts exist | Go constant `Protocol` in `server/internal/server/protocol.go` (Arch-server, contracts.md) | `GET /healthz` header `Agent-Bus-Protocol`; extension compares with `MIN_SERVER_PROTOCOL` |

### 3.1 Plugin version

- `extension/lib/version/plugin-version.ts` reads `package.json` next to the package root (`new URL("../../../package.json", import.meta.url)` from `extension/lib/version/`) once at load: `createBusExtension` starts the read in its closure, which every session of the process shares. No second constant (findings R2).
- The root `package.json` is always present in a git-dependency install (it is the manifest omp reads: `pkg.omp ?? pkg.pi`, `CA/extensibility/plugins/installer.ts:70`).

### 3.2 Server protocol

- The extension declares `MIN_SERVER_PROTOCOL` in `extension/lib/version/server-protocol.ts`. It is raised in the **same change** that makes the extension depend on a newer server contract (E9a set 2; E6/E8b needed no bump, because their server slices shipped under protocol 2, `contracts.md` §6).
- The server constant is bumped by Arch-server's slices (≤ once per release; history comment per value).

### 3.3 Fitness checks

| Check | Where | Fails when |
|---|---|---|
| `tests/version/protocol-sync.test.ts` | bun, `ci-typescript` and `full` | `MIN_SERVER_PROTOCOL` > the `Protocol` constant parsed from `server/internal/server/protocol.go` (a plugin at HEAD would refuse the server at HEAD) |
| `scripts/release_version.py` + `tests/test_release_version.py` | python, `release.yml`, `ci-python` and `full` | the tag is no `vMAJOR.MINOR.PATCH[-PRERELEASE]` SemVer string or ≠ `package.json` version; message `package version 0.1.0 does not match tag v0.2.0` (`plugin-version-skew.feature:88-91`) |
| `tests/layout.test.ts` | bun | an import edge outside `client.md` §1.3 |

`scripts/release_version.py` is its own file (one responsibility; `build.py` stays "build"):

```python
def package_version(package_json: Path) -> str: ...
def check(tag: str, version: str) -> str | None:  # error message or None
def main(argv: list[str] | None = None) -> int:   # --tag "$GITHUB_REF_NAME"
```

---

## 4. Release: version bump and tag check

### 4.1 Workflow (`release.yml`)

`release.yml` is the publisher: it runs no test suite (the full tier did, §2).

```yaml
on:
  push:
    tags: ['v*']
permissions:
  contents: write
  actions: read          # list the full-tier runs of the tagged commit
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-python@v7
        with: { python-version: '3.x' }
      - name: Tag matches package version
        run: python scripts/release_version.py --tag "$GITHUB_REF_NAME"
      - name: Full tier passed for the tagged commit
        env: { GH_TOKEN: ${{ github.token }} }
        run: |
          gh api "repos/${GITHUB_REPOSITORY}/actions/workflows/full.yml/runs?head_sha=${GITHUB_SHA}&event=push&per_page=100" \
            | python scripts/release_ci_gate.py --sha "$GITHUB_SHA"
      - uses: actions/setup-go@v7
        with: { go-version-file: server/go.mod, cache-dependency-path: server/go.sum }
      - uses: oven-sh/setup-bun@v2
        with: { bun-version-file: gui/package.json }
      - name: Build release binaries (GUI first)
        run: python scripts/build.py --version "${GITHUB_REF_NAME#v}"
      - name: Create GitHub Release
        run: gh release create … (`--prerelease` when the version contains `-`)
```

The two checks run first, so a bad tag fails in seconds and publishes nothing:

- **Version check** (`scripts/release_version.py`): the tag is `v` plus a SemVer 2.0.0 version `MAJOR.MINOR.PATCH[-PRERELEASE]` (no leading zeros, pre-release identifiers alphanumeric or numeric without leading zeros, e.g. `v0.5.0-rc.1`, `v3.5.1-prerelease01`; `v3.5.1-01`, `v1.2`, `1.2.3` fail) and equals the `package.json` version exactly. Build metadata (`+build`) is refused: precedence ignores it and it is awkward in asset names. A `v*` tag publishes a release, hence the strictness; any other tag fails in this first step with the expected form. It needs no network.
- **Full-tier gate** (`scripts/release_ci_gate.py`, pytest `tests/test_release_ci_gate.py`): it requires a `push` run of the `full` workflow (`full.yml`) for the tagged commit that concluded `success`. `pull_request` runs do not count (the full tier has none; the filter keeps a broken query from passing), and the script drops runs of other commits. One successful run is enough: normally the run of the merge to `main`, otherwise the run of the tag push itself (when no run of `main` covered the commit, `full.yml` runs the suites on the tag). The tag push starts its own `full` run, which may still be in progress while the release checks. No run, only failed runs or only running runs → exit 1 with the commit and the listed runs; re-run the release workflow once the full tier passed.

### 4.2 Release procedure (documented in the root `README.md` "Releasing" section)

1. Branch `release/X.Y.Z` from `develop` (Git Flow; SemVer: a release from `develop` bumps MINOR, or MAJOR for breaking changes, and while MAJOR is 0 anything may change on MINOR; the tag must equal `package.json` `version`): bump `package.json` `version` (R1); if a server contract the extension needs changed in this release, check `protocol.go` is at least `MIN_SERVER_PROTOCOL` (the fitness test enforces `MIN_SERVER_PROTOCOL` ≤ `Protocol`). Only stabilizing fixes after that. Local checks. A release candidate is tagged on the release branch with `package.json` at `X.Y.Z-rc.N` (`vX.Y.Z-rc.N`); `release.yml` creates it with `--prerelease`, so it never becomes `releases/latest`, which the extension's `/bus server start` downloads (`extension/lib/local-server/download.ts`, `RELEASES_URL`). No push to `main` covers a release-branch commit, so the full-tier run of the candidate's tag push runs the suites itself (`covered` finds nothing) and the release workflow, started at the same time, fails its gate: re-run it after the full-tier run passed. Cheapest correct option: no extra trigger, no polling job, no repeated suites; the cost is one manual re-run per candidate. The final tag `vX.Y.Z` is on the merge commit on `main`, which the push to `main` already covered.
2. Merge `release/X.Y.Z` into `main` with a merge commit and push: that starts `full.yml`. When it is green, tag the merge commit `vX.Y.Z`, push the tag, merge the release branch back into `develop` → `release.yml` checks the version and the full-tier run, builds GUI + 6 server binaries + `SHA256SUMS`, publishes the GitHub Release.
3. A `hotfix/<name>` branch (from `main`, PATCH bump in `package.json`) takes the same way: merge into `main`, full tier, tag `vX.Y.Z` with the new patch, merge into `develop`.
4. Users: `omp plugin upgrade @till-muellers/omp-agent-message-bus` (the targeted form; findings R4) and replace the server binary. README states both.
5. Plugin and server of one tag are compatible: the `protocol-sync` test (`tests/version/protocol-sync.test.ts`) checks that the server's `Protocol` at HEAD is at least the extension's `MIN_SERVER_PROTOCOL` (≥, not equal). Mixed pairs are caught at runtime by the extension's protocol gate (`client.md` §8.1) and the spawn skew check (§8.2).

No npm publish, no registry token. The plugin needs no build step (Bun loads `.ts` directly).

---

## 5. Root `package.json`

| Field | Change | Slice |
|---|---|---|
| `version` (:3) | bumped per release (0.1.0 → 0.2.0 for the first release with these changes) | P2b |
| `files` (:28-33) | drop `"skills"` | E8c (with the folder deletion; see `client.md` §9) |
| `scripts` (:34-37) | add `"test:gui": "bun test --cwd gui"`, `"build:gui": "bun run --cwd gui build"`; `test` and `typecheck` unchanged. The `web` jobs (`ci-web`, `full`) run `test:gui` and `build:gui`, so they cannot rot (§2) | GUI-1 |
| `omp` (:23-27) | unchanged (`./extension/index.ts`) | — |
| `devDependencies` | unchanged | — |

`gui/` is not in `files`. `[INFERENCE: a `github:` dependency is installed from the repo tarball, so `files` may not filter it; harmless either way because nothing under `gui/` is referenced by `omp.extensions`.]`

---

## 6. Removing `skills/` from the package

- E8c deletes `skills/agent-message-bus/SKILL.md` and `skills/`, and drops `"skills"` from `files`. No workflow, script or test references the skill (scout grep over `.github/`, `scripts/`, `tests/`).
- omp discovers plugin skills from `<package root>/skills` by convention only (`CA/discovery/omp-plugins.ts:65-79`; no manifest key, `CA/extensibility/plugins/types.ts:29-52`), so an upgraded install has no skill; an old install keeps it until `omp plugin upgrade` `[INFERENCE]`.
- Docs listed in `client.md` §9.

---

## 7. Open pipeline questions

Consolidated in `QUESTIONS.md` (recommendation, rationale, what each blocks). Defaults used by the slices:

| # | Question | Default if unanswered |
|---|---|---|
| Q-P1 | Self-contained Docker GUI build with an `oven/bun:1` stage, or build the GUI before `docker build`? | Variant A: build the GUI on the host first, Dockerfile unchanged, no new image (PL-2 docs only). |
| Q-P2 | Make `web` a required status check on `main`? | Superseded by Git Flow: `ci-web` is path-filtered, and a skipped required check blocks a merge, so no cheap-tier job is required. If any check is required on `main`, it is a full-tier job (`full.yml` runs only on `main` and tags, so it cannot gate the merge itself). Owner decision in repo settings. |
| Q-P3 | Pin bun in CI (`bun-version`) to the version used locally? | Pin to the version in `gui/bun.lock`'s generator once the first lockfile exists (GUI-1). Done in GUI-1: `bun.lock` records no bun version, so the pin is `gui/package.json` `packageManager: "bun@1.4.2"` (the bun that wrote the first lockfile), read by `setup-bun` through `bun-version-file: gui/package.json` in the TypeScript and web jobs of the cheap and the full tier and in `release.yml`. |
