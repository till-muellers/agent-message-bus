# Pipeline architecture: CI, release, build, Docker, root package.json

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`.

Owner: Arch-client. Files: `.github/workflows/ci-go.yml`, `ci-typescript.yml`, `ci-web.yml`, `ci-python.yml` (reduced suite, §2), `full.yml` (full suite, §2), `release.yml` (publisher, §4), `scripts/build.py` (GUI build, then Go cross-compile for the targets), `server/Dockerfile` (context `server/`), root `package.json` (`version`, `omp.extensions`, `files`). The branch model they serve is Git Flow (`.omp/AGENTS.md` "Git workflow and CI").
Distribution: **GitHub only, no npm** (findings item 8, PO round 2). PO decisions: PO-10 (GUI build before `go build`, committed placeholder), PO-11 (S1 spawn-time version check). `CA/` = `node_modules/@oh-my-pi/pi-coding-agent/src/` (omp 18.12.6). Server-side contracts: `docs/design/arch/contracts.md` §6 (protocol), §10.5 (static assets).

---

## 1. GUI build before `go build`

The Go binary embeds `server/internal/web/dist/` (Arch-server owns `embed.go`, `dist/.gitkeep`, `server/internal/web/.gitignore` and the fallback page, contracts.md §10.5). Every place that produces a **release** binary must run the GUI build first; every place that only **tests** Go works without it (the fallback page is served).

| Place | Change |
|---|---|
| `scripts/build.py` | New step before the target loop: `build_gui(out_dir=APP_DIR / "internal/web/dist")` runs `bun install --frozen-lockfile` and `bun run build` in `gui/`. `find_bun()` mirrors `find_go()` (:50-61). Flag `--skip-gui` for Go-only builds (local cross-compile without bun); without the flag a missing bun is an error, so a release can never ship the fallback page silently. The GUI build runs once, not per target. |
| `.github/workflows/release.yml`, `full.yml` (`package`) | `oven-sh/setup-bun@v2` before "Build release binaries"; build.py then builds the GUI. |
| `server/Dockerfile` | **Variant A (recommended, Q-P1, PL-2): unchanged.** The context stays `server/`; the GUI is built on the host first (`bun run build:gui`), and the existing `COPY . .` carries `internal/web/dist` into the image (`server/.dockerignore` excludes only `.git`, `*.db`, `data/`). Without the GUI step the image serves the fallback page. Docs: `server/DEPLOYMENT.md:146`, `server/README.md:39`. No image is published by CI (`release.yml` builds binaries only), so no workflow changes. **Variant B (only with Q-P1 approval):** new first stage `FROM oven/bun:1 AS gui`: `COPY gui/ /src/gui/`, `bun install --frozen-lockfile`, `bun run build --out /out/dist`; Go stage: `COPY server/ .` and `COPY --from=gui /out/dist internal/web/dist`; build context = repo root (`docker build -f server/Dockerfile -t agent-message-bus .`); root `.dockerignore` (`node_modules`, `tmp`, `.git`, `dist`, `.beads`, `.dolt`). `oven/bun` is a new base image and needs approval under "no new dependency without asking". |
| `.github/workflows/ci-web.yml`, `full.yml` | The `web` jobs (§2). The `go` jobs stay bun-free and test against the fallback page. |
| Local dev | `go build` / `go test` need nothing new. GUI work: `bun run --cwd gui build`; the output is ignored by `server/internal/web/.gitignore`. |
| `.gitignore` | `dist/` → `/dist/` (keeps ignoring the release output at the repo root, stops hiding `server/internal/web/dist/.gitkeep`). Lands in server slice W1a with the `.gitkeep` (requested from Arch-server). |

`build.py` functions stay single-purpose and typed (`.omp/RULES.md`); pytest in `tests/test_build.py` covers `gui_build_commands()` (pure: returns the two argv lists and cwd), `--skip-gui` parsing and `find_bun()` with a patched `shutil.which`.

---

## 2. CI: pull requests only, the local checks are the gate

CI costs money (billed per minute), so the regular gate is the local checks (`.omp/AGENTS.md`, "Git workflow and CI"). CI runs on pull requests only, on the merge result (GitHub's default `pull_request` ref), never on a branch push: a feature branch without a pull request costs nothing. Every workflow runs on `ubuntu-latest`: we develop on Windows, so Windows is covered by the local runs, and a Linux-only failure is reproduced locally in WSL (command in `.omp/AGENTS.md`).

The target branch of the pull request picks the suite. The triggers, jobs and steps are in the workflow files; this table lists only what is not obvious from them:

| Workflow | Pull request into | Why these paths |
|---|---|---|
| `ci-go.yml` | `develop`, `release/**`, `hotfix/**` | `server/**` |
| `ci-typescript.yml` | same | the extension and its tests, plus files the tests read: `server/TOOLS.md`, `server/internal/server/protocol.go`, the bun pin `gui/package.json`, the workflows (`tests/workflows.test.ts`) |
| `ci-web.yml` | same | `gui/**` and the contract fixtures `server/internal/web/testdata/**` that `gui/tests/api/contract.test.ts` decodes |
| `ci-python.yml` | same | `scripts/**`, `tests/test_*.py`, `ruff.toml` |
| `full.yml` | `main` | none: always everything |
| `release.yml` | a `v*` tag push, no pull request | none |

- **Reduced suite** (`ci-*.yml`): build, unit tests and linters of the area a pull request touches, without `-race`. A change outside every filter (docs, `.omp/`, `.beads/`) starts no job. Its jobs are never required checks: a skipped path-filtered required check blocks a merge.
- **Full suite** (`full.yml`): never path-filtered, so its jobs `full-go`, `full-typescript`, `full-web`, `full-python`, `full-package` always report and are the required checks of `main`; their names are unique across the workflows. It adds the race detector (needs CGO; the runner has gcc), every area on every run, the Go build against the real GUI bundle (`go build ./...`, `go test ./internal/web/...` after `bun run build:gui`) and the packaging smoke build (`scripts/build.py`, four binaries and `SHA256SUMS`, discarded). It runs on the merge result of every pull request into `main` (release finish, hotfix finish, `develop` → `main`), and nothing else reaches `main`, so a release rests on a green full suite; `release.yml` neither repeats nor consults it (§4).
- Drafts are skipped and a newer commit cancels the run of the same workflow and pull request. A pull request whose base is changed to `main` after it was opened gets no full run (the `edited` type is not listened to, so title edits cost nothing): close and reopen it or push a commit. `workflow_dispatch` on the reduced and the full workflows is the manual escape hatch (the file must exist on the default branch to be dispatchable), for example the full suite on a release branch before a release candidate tag.
- Tools are pinned: bun through `bun-version-file: gui/package.json` (Q-P3), ruff `==0.17.3` (what `ruff.toml` requires), Go through `server/go.mod`. No third-party action beyond `actions/*`, `oven-sh/setup-bun` and `golangci/golangci-lint-action`.
- The `web` jobs run the GUI tests and build through the root scripts `test:gui` and `build:gui` (§5), so contributors run locally what CI runs. `tests/package.test.ts` scans every tracked text file; the reduced suite runs it only when a TypeScript-area path changed, the full suite always.
- `tests/workflows.test.ts` keeps these rules from rotting (triggers, paths, draft guard, concurrency, runner, actions, publisher).

### Repository settings (owner, one time, outside the code)

- Default branch stays `main`: `omp plugin install`/`upgrade` and the repository page then show released states, not `develop`. Pull requests into `develop` name it as the base explicitly.
- `main`: require a pull request (no direct push, no force push, no deletion); required status checks `full-go`, `full-typescript`, `full-web`, `full-python`, `full-package`; require branches to be up to date before merging (so the checked merge result is the merged result); allow merge commits only (no squash, no rebase), so release history stays connected to `develop`. The pull request into `main` is the release/hotfix finish: release candidates are tagged on the release branch, the final tag on the merge commit.
- `develop`: pull requests only (merge commits), no force push, no deletion. Required checks optional: the reduced jobs are path-filtered and must not be required (a skipped required check blocks the merge).
- Remove the old required checks `go`, `typescript`, `web`, `python` (those names no longer exist) and drop the planned required `web` check (Q-P2).

### Cost per typical change (estimate, job runs)

|Step|Before (`ci.yml`)|After|
|---|---|---|
|Feature through a pull request into `develop` (3 pushes)|6 runs x 4 jobs (every push and every PR update, no cancel, no filter, `-race`)|3 runs x 1-2 jobs (only the touched areas, superseded runs cancelled, no `-race`)|
|Docs-only change|4 jobs per push|0|
|Release|`main` run (4 jobs), tag run (4 jobs), `go test` again in `release.yml`|one pull request into `main`: 5 full jobs per push to it; the tag only publishes (1 job)|

Per-change cost falls to a third or less for pull requests; only the pull request into `main`, once per release, runs everything.

---

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
| `tests/version/protocol-sync.test.ts` | bun, `ci-typescript` and `full-typescript` | `MIN_SERVER_PROTOCOL` > the `Protocol` constant parsed from `server/internal/server/protocol.go` (a plugin at HEAD would refuse the server at HEAD) |
| `scripts/release_version.py` + `tests/test_release_version.py` | python, `release.yml`, `ci-python` and `full-python` | the tag is no `vMAJOR.MINOR.PATCH[-PRERELEASE]` SemVer string or ≠ `package.json` version; message `package version 0.1.0 does not match tag v0.2.0` (`plugin-version-skew.feature:88-91`) |
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

`release.yml` is a plain publisher: it runs no suite and consults no earlier CI run. The full suite already passed on the pull request into `main`, and the required checks of `main` (§2 "Repository settings") guarantee that nothing reached `main` without it; that is a one-time repository setting, not a per-release step.

Steps (`release.yml`): the version check, then `scripts/build.py --version <tag without v>` (GUI first), then `gh release create` (`--prerelease` when the version contains `-`).

The version check runs first, so a bad tag fails in seconds and publishes nothing (`scripts/release_version.py`, pytest `tests/test_release_version.py`): the tag is `v` plus a SemVer 2.0.0 version `MAJOR.MINOR.PATCH[-PRERELEASE]` (no leading zeros, pre-release identifiers alphanumeric or numeric without leading zeros, e.g. `v0.5.0-rc.1`, `v3.5.1-prerelease01`; `v3.5.1-01`, `v1.2`, `1.2.3` fail) and equals the `package.json` version exactly. Build metadata (`+build`) is refused: precedence ignores it and it is awkward in asset names. A `v*` tag publishes a release, hence the strictness; any other tag fails in this first step with the expected form. It needs no network.

### 4.2 Release procedure (documented in the root `README.md` "Releasing" section)

1. Branch `release/X.Y.Z` from `develop` (Git Flow; SemVer: a release from `develop` bumps MINOR, or MAJOR for breaking changes, and while MAJOR is 0 anything may change on MINOR; the tag must equal `package.json` `version`): bump `package.json` `version` (R1); if a server contract the extension needs changed in this release, check `protocol.go` is at least `MIN_SERVER_PROTOCOL` (the fitness test enforces `MIN_SERVER_PROTOCOL` ≤ `Protocol`). Only stabilizing fixes after that.
2. Optional release candidate: `package.json` at `X.Y.Z-rc.N` on the release branch, tag `vX.Y.Z-rc.N` on that commit. `release.yml` creates it with `--prerelease`, so it never becomes `releases/latest`, which the extension's `/bus server start` downloads (`extension/lib/local-server/download.ts`, `RELEASES_URL`). The assurance for a candidate is the reduced suite of the pull requests into the release branch plus the local checks. To run everything before the tag, dispatch `full.yml` on the release branch by hand (`workflow_dispatch`, available once `full.yml` is on `main`); nothing requires it.
3. Pull request `release/X.Y.Z` → `main`: the full suite runs on its merge result and is a required check. The last commit of the release branch sets the final `X.Y.Z` in `package.json`. Merge with a merge commit, tag the merge commit `vX.Y.Z`, push the tag, then merge `main` back into `develop` by pull request. `release.yml` checks the version, builds GUI + 4 server binaries + `SHA256SUMS` and publishes the GitHub Release.
4. A `hotfix/<name>` branch (from `main`, PATCH bump in `package.json`) takes the same way: pull request into `main` (full suite), tag `vX.Y.Z` with the new patch version, pull request `main` into `develop`.
5. Users: `omp plugin upgrade @till-muellers/omp-agent-message-bus` (the targeted form; findings R4) and replace the server binary. README states both.
6. Plugin and server of one tag are compatible: the `protocol-sync` test (`tests/version/protocol-sync.test.ts`) checks that the server's `Protocol` at HEAD is at least the extension's `MIN_SERVER_PROTOCOL` (≥, not equal). Mixed pairs are caught at runtime by the extension's protocol gate (`client.md` §8.1) and the spawn skew check (§8.2).

No npm publish, no registry token. The plugin needs no build step (Bun loads `.ts` directly).

---

## 5. Root `package.json`

| Field | Change | Slice |
|---|---|---|
| `version` (:3) | bumped per release (0.1.0 → 0.2.0 for the first release with these changes) | P2b |
| `files` (:28-33) | drop `"skills"` | E8c (with the folder deletion; see `client.md` §9) |
| `scripts` (:34-37) | add `"test:gui": "bun test --cwd gui"`, `"build:gui": "bun run --cwd gui build"`; `test` and `typecheck` unchanged. The `web` jobs (`ci-web`, `full-web`) run `test:gui` and `build:gui`, so they cannot rot (§2) | GUI-1 |
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
| Q-P2 | Make `web` a required status check on `main`? | Superseded by Git Flow: `ci-web` is path-filtered, and a skipped required check blocks a merge, so no reduced-suite job is required. `main` requires the five `full-*` jobs of the pull request into it (§2 "Repository settings"), among them `full-web`. |
| Q-P3 | Pin bun in CI (`bun-version`) to the version used locally? | Pin to the version in `gui/bun.lock`'s generator once the first lockfile exists (GUI-1). Done in GUI-1: `bun.lock` records no bun version, so the pin is `gui/package.json` `packageManager: "bun@1.4.2"` (the bun that wrote the first lockfile), read by `setup-bun` through `bun-version-file: gui/package.json` in the TypeScript and web jobs of the reduced and the full suite and in `release.yml`. |
