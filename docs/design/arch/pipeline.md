# Pipeline architecture: CI, release, build, Docker, root package.json

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`.

Owner: Arch-client. Today's files: `.github/workflows/ci.yml` (jobs `go` :11-37, `typescript` :39-53, `python` :55-68), `.github/workflows/release.yml` (tag `v*` :4-5; go test :21-23; `python scripts/build.py --version` :29-30; `gh release create … dist/*` :32-39), `scripts/build.py` (Go cross-compile only, `build_target` :64-71, `main` :96-108), `server/Dockerfile` (context `server/`, Go build :6), root `package.json` (`version` 0.1.0 :3, `omp.extensions` :23-27, `files` with `skills` :28-33).
Distribution: **GitHub only, no npm** (findings item 8, PO round 2). PO decisions: PO-10 (GUI build before `go build`, committed placeholder), PO-11 (S1 spawn-time version check). `CA/` = `node_modules/@oh-my-pi/pi-coding-agent/src/` (omp 18.12.6). Server-side contracts: `docs/design/arch/contracts.md` §6 (protocol), §10.5 (static assets).

---

## 1. GUI build before `go build`

The Go binary embeds `server/internal/web/dist/` (Arch-server owns `embed.go`, `dist/.gitkeep`, `server/internal/web/.gitignore` and the fallback page, contracts.md §10.5). Every place that produces a **release** binary must run the GUI build first; every place that only **tests** Go works without it (the fallback page is served).

| Place | Change |
|---|---|
| `scripts/build.py` | New step before the target loop: `build_gui(out_dir=APP_DIR / "internal/web/dist")` runs `bun install --frozen-lockfile` and `bun run build` in `gui/`. `find_bun()` mirrors `find_go()` (:50-61). Flag `--skip-gui` for Go-only builds (local cross-compile without bun); without the flag a missing bun is an error, so a release can never ship the fallback page silently. The GUI build runs once, not per target. |
| `.github/workflows/release.yml` | `oven-sh/setup-bun@v2` before "Build release binaries"; build.py then builds the GUI. |
| `server/Dockerfile` | **Variant A (recommended, Q-P1, PL-2): unchanged.** The context stays `server/`; the GUI is built on the host first (`bun run build:gui`), and the existing `COPY . .` carries `internal/web/dist` into the image (`server/.dockerignore` excludes only `.git`, `*.db`, `data/`). Without the GUI step the image serves the fallback page. Docs: `server/DEPLOYMENT.md:146`, `server/README.md:39`. No image is published by CI (`release.yml` builds binaries only), so no workflow changes. **Variant B (only with Q-P1 approval):** new first stage `FROM oven/bun:1 AS gui`: `COPY gui/ /src/gui/`, `bun install --frozen-lockfile`, `bun run build --out /out/dist`; Go stage: `COPY server/ .` and `COPY --from=gui /out/dist internal/web/dist`; build context = repo root (`docker build -f server/Dockerfile -t agent-message-bus .`); root `.dockerignore` (`node_modules`, `tmp`, `.git`, `dist`, `.beads`, `.dolt`). `oven/bun` is a new base image and needs approval under "no new dependency without asking". |
| `.github/workflows/ci.yml` | New job `web` (§2). The `go` job stays bun-free and tests against the fallback page. |
| Local dev | `go build` / `go test` need nothing new. GUI work: `bun run --cwd gui build`; the output is ignored by `server/internal/web/.gitignore`. |
| `.gitignore` | `dist/` → `/dist/` (keeps ignoring the release output at the repo root, stops hiding `server/internal/web/dist/.gitkeep`). Lands in server slice W1a with the `.gitkeep` (requested from Arch-server). |

`build.py` functions stay single-purpose and typed (`.omp/RULES.md`); pytest in `tests/test_build.py` covers `gui_build_commands()` (pure: returns the two argv lists and cwd), `--skip-gui` parsing and `find_bun()` with a patched `shutil.which`.

---

## 2. CI (`.github/workflows/ci.yml`)

```yaml
  web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: oven-sh/setup-bun@v2
      - uses: actions/setup-go@v7
        with: { go-version-file: server/go.mod, cache-dependency-path: server/go.sum }
      - name: Install GUI
        run: bun install --frozen-lockfile
        working-directory: gui
      - name: Typecheck GUI
        run: bun run typecheck
        working-directory: gui
      - name: Test GUI
        run: bun test
        working-directory: gui
      - name: Build GUI into the embed dir
        run: bun run build
        working-directory: gui
      - name: Go build and web tests with the real bundle
        run: go build ./... && go test ./internal/web/...
        working-directory: server
```

- Existing `typescript` job (:39-53) is unchanged except for one new test file (§3.3, `tests/version/protocol-sync.test.ts`), which runs inside `bun test tests/`.
- Existing `python` job (:55-68): `python -m pytest tests/test_build.py tests/test_release_version.py`.
- `.omp/AGENTS.md` "Definition of done" gains `cd gui && bun test && bun run typecheck` and the PR's required checks gain `web` (repo settings, outside the code: ask the owner).

---

## 3. Version and protocol: sources of truth

There are **two** numbers, and they must not be confused.

| Number | Meaning | Single source | Read by |
|---|---|---|---|
| Plugin version (semver, e.g. `0.2.0`) | which extension code a copy holds; "extension protocol version" in `plugin-version-skew.feature` | root `package.json` `version` (:3) | `omp plugin list` (manifest of the installed commit); `extension/lib/version/plugin-version.ts` at load; release tag check |
| Server protocol (integer, e.g. `2`) | which server contracts exist | Go constant `Protocol` in `server/internal/server/protocol.go` (Arch-server, contracts.md) | `GET /healthz` header `Agent-Bus-Protocol`; extension compares with `MIN_SERVER_PROTOCOL` |

### 3.1 Plugin version

- `extension/lib/version/plugin-version.ts` reads `package.json` next to the package root (`new URL("../../../package.json", import.meta.url)` from `extension/lib/version/`) once at load. No second constant (findings R2).
- The root `package.json` is always present in a git-dependency install (it is the manifest omp reads: `pkg.omp ?? pkg.pi`, `CA/extensibility/plugins/installer.ts:70`).

### 3.2 Server protocol

- The extension declares `MIN_SERVER_PROTOCOL` in `extension/lib/version/server-protocol.ts`. It is raised in the **same PR** that makes the extension depend on a newer server contract (E9a sets 2; the release with E6/E8b sets 3, per contracts.md).
- The server constant is bumped by Arch-server's slices (≤ once per release; history comment per value).

### 3.3 Fitness checks

| Check | Where | Fails when |
|---|---|---|
| `tests/version/protocol-sync.test.ts` | bun, `typescript` job | `MIN_SERVER_PROTOCOL` > the `Protocol` constant parsed from `server/internal/server/protocol.go` (a plugin at HEAD would refuse the server at HEAD) |
| `scripts/release_version.py` + `tests/test_release_version.py` | python, `release.yml` and `python` job | the tag `vX.Y.Z` ≠ `package.json` version; message `package version 0.1.0 does not match tag v0.2.0` (`plugin-version-skew.feature:88-91`) |
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

```yaml
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-python@v7
        with: { python-version: '3.x' }
      - name: Tag matches package version
        run: python scripts/release_version.py --tag "$GITHUB_REF_NAME"
      - uses: actions/setup-go@v7
        with: { go-version-file: server/go.mod, cache-dependency-path: server/go.sum }
      - uses: oven-sh/setup-bun@v2
      - name: Test
        working-directory: server
        run: go test ./...
      - name: Build release binaries (GUI first)
        run: python scripts/build.py --version "${GITHUB_REF_NAME#v}"
      - name: Create GitHub Release
        run: gh release create … (unchanged, notes gain the "delete pre-release databases" line of REVIEW P2 for the reset release)
```

The version check runs first, so a mismatched tag fails in seconds and publishes nothing.

### 4.2 Release procedure (documented in the root `README.md` "Release" section)

1. Branch `release/vX.Y.Z`: bump `package.json` `version` (R1); if a server contract the extension needs changed in this release, check `protocol.go` and `MIN_SERVER_PROTOCOL` agree (the fitness test enforces ≤). PR → green CI → merge (`main` is protected).
2. Tag the merge commit `vX.Y.Z` and push the tag → `release.yml` checks, builds GUI + 6 server binaries + `SHA256SUMS`, publishes the GitHub Release.
3. Users: `omp plugin upgrade @till-muellers/omp-agent-message-bus` (the targeted form; findings R4) and replace the server binary. README states both.
4. Plugin and server of one tag always match (`protocol-sync` test). Mixed pairs are caught at runtime by the extension's protocol gate (`client.md` §8.1) and the spawn skew check (§8.2).

No npm publish, no registry token. The plugin needs no build step (Bun loads `.ts` directly).

---

## 5. Root `package.json`

| Field | Change | Slice |
|---|---|---|
| `version` (:3) | bumped per release (0.1.0 → 0.2.0 for the first release with these changes) | P2b |
| `files` (:28-33) | drop `"skills"` | E8c (with the folder deletion; see `client.md` §9) |
| `scripts` (:34-37) | add `"test:gui": "bun test --cwd gui"`, `"build:gui": "bun run --cwd gui build"` as conveniences; `test` and `typecheck` unchanged | GUI-1 |
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
| Q-P2 | Make `web` a required status check on `main`? | Yes (owner action in repo settings, after GUI-1). |
| Q-P3 | Pin bun in CI (`bun-version`) to the version used locally? | Pin to the version in `gui/bun.lock`'s generator once the first lockfile exists (GUI-1). |
