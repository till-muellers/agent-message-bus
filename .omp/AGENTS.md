# Project guide — agent-message-bus

Conventions and working method for contributors and coding agents in this repo.

## Stack
- Server: Go (`server/`), SQLite embedded via pure-Go `modernc.org/sqlite`, MCP via `github.com/modelcontextprotocol/go-sdk`. One static binary, `CGO_ENABLED=0`.
- omp plugin: TypeScript on Bun (`extension/`), typechecked with `tsc --noEmit`.
- Web GUI: strict TypeScript on Bun without a framework (`gui/`, its own `package.json` and `tsconfig.json`), bundled into `server/internal/web/dist/` and embedded by the server.
- Python: release tooling only (`scripts/build.py`, `scripts/release_version.py`).
- CI: GitHub Actions in `.github/workflows/`, on pull requests only: the reduced suite `ci-go.yml`, `ci-typescript.yml`, `ci-web.yml`, `ci-python.yml`, the full suite `full.yml`; the tag publisher `release.yml` (see "Git Flow and CI").

## Layout

|Path|Contents|
|---|---|
|`server/`|Go server. `main.go`, `cmd/tooldoc/`, `internal/`, `migrations/`, `spec/`.|
|`extension/`|omp extension (TypeScript). `index.ts` wires `lib/<feature>/`; `lib/shared/` holds generic helpers no feature owns (command argument parsing and the `CommandResult` shape, the owner-only credential file store).|
|`tests/`|Per-module `*.test.ts` (bun) mirroring the `extension/lib/<feature>/` folders, with the shared `fakes.ts` and the launcher tests in `launch/`, plus `test_*.py` (pytest) for `scripts/`.|
|`gui/`|Web GUI: `src/` (browser code), `tests/` (bun), `scripts/build.ts` (bundles into `server/internal/web/dist/`), `index.html` (shell template).|
|`scripts/build.py`|Release build: builds the GUI (`--skip-gui` to skip), cross-compiles the server and writes `SHA256SUMS`.|
|`scripts/release_version.py`|First step of `release.yml`: the tag is a SemVer version `v<MAJOR.MINOR.PATCH[-PRERELEASE]>` equal to the `package.json` version.|
|`docs/design/`|Committed design: `specs/` is the frozen input (the scenarios move to `server/spec/` when their slice is implemented); `arch/` holds the architecture, of which `contracts.md` and `BACKLOG.md` stay living and change only in the feature that changes a contract or the backlog.|
|`.omp/`|This guide and the hard rules (`RULES.md`).|

## Commands
- Go test: `cd server && go test ./...` (single pkg: `go test ./internal/<feature>/...`, single test: `go test -run TestName ./path`)
- Go race + cover: `go test -race -cover ./...` (needs CGO; part of the local checks; only the full suite of CI runs it)
- Go lint/format: `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`
- TS: `bun test tests/`, `bun run typecheck`
- GUI: `cd gui && bun test && bun run typecheck`; build into the embed dir with `bun run build:gui` (repo root). Go builds and tests work without it (fallback page at `/web/`).
- Python: `pytest tests`, `ruff format --check .`, `ruff check .` (settings in `ruff.toml`, which requires ruff 0.17.3 like CI, e.g. `uvx ruff@0.17.3 check .`; `ruff format .` to fix)
- Tool ledger: `cd server && go run ./cmd/tooldoc > TOOLS.md`

## Architecture
- `server/main.go`: wiring and CLI dispatch only, no logic: config, CLI dispatch, DB, migrations, `server.Serve`.
- `server/internal/<feature>/`: one folder per feature (`auth`, `inbox`, `spaces`, `invites`, `messaging`, `blackboards`, `overview`). `internal/guide` is not a feature: it holds only the embedded guide text (below).
  - `service.go`: business logic
  - `repository.go`: all SQL of the feature, as package functions over `db.DBTX` (no repository interfaces)
  - `*_test.go`: table-driven tests next to the code, real SQLite via `internal/platform/db/dbtest`
- **Features own their tables.** Each feature writes only its own tables. Cross-feature effects go through hooks (`spaces.CreationHook`, `spaces.DepartureHook`, `spaces.RevocationHook`, passed together in the `spaces.Hooks` parameter object) and service methods, inside the caller's transaction: the feature that runs the operation opens it (`db.InTx`, or `inbox.InTx` when it produces inbox entries) and passes it on as `db.DBTX`. `internal/overview` is a read composition (the space overview and the web GUI state): it owns no table, resolves the caller once and reads only through the spaces, messaging, blackboards, invites and inbox services; only `internal/tools`, `internal/web` and the `internal/server` wiring import it. Rights rules live in `spaces.Membership` (`Leads`, `Joined`, `MaySeeInvites`); transports ask them, never re-implement them.
- Transports are thin: `internal/tools` (the handler layer: one registry of `bus_*` tools), `internal/rpc`, `internal/mcpserver`, `internal/admin`, `internal/web` (the web GUI backend: embedded app, cookie sessions re-validated per request; served only on its own listener `AMB_WEB_ADDR`, never on the API listener); `internal/server` composes everything (`Build` returns the API and GUI handlers, `Routes`/`WebRoutes`, `Run` serves both listeners, `Serve` builds, opens the listeners, publishes the instance and runs).
- `internal/guide`: the agent guide (`guide.md`, embedded, ≤ 4000 characters, the same for every token). It is served as the MCP `instructions`, as `GET /v1/guide` and as the TOOLS.md section "Working on the bus"; edit `guide.md`, then regenerate TOOLS.md.
- `internal/domain/`: business rules shared by features (names, body and title limits).
- `internal/platform/`: infrastructure that is not business logic (apperr, db, broker, httpx, httpserver, config, instance, detach). Some packages have a single consumer (`broker`, `httpserver`, `detach`): they live here because they are infrastructure, not because they are shared.
- `server/migrations/`: numbered SQL files; schema never inline in Go code.
- Dependencies point inward: transport → service → repository/platform. Never the reverse.
- Behaviour is specified in `server/spec/*.feature`; keep it in sync.

## Workflow
### Understand the specs
- Ask if requirements are unclear.
- Map the specs on the codebase. Try to answer these questions:
	- WHAT do I have to change or extend? Is this a new feature?
	- Is there an EXTENSION SEAM meant for my task (e.g. `spaces.CreationHook`/`DepartureHook`/`RevocationHook` via `spaces.Hooks`, the tool registry, `inbox.InTx` with `inbox.Sender`)?
		- What pattern does it follow (strategy, hook, decorator, ...)?
		- Was it really made for tasks like mine? Is there an alternative?
		- Does it provide the required abstractions, or does it have to be expanded?
	- Is there code similar to my use case? What can I learn from it?
- Ask before introducing, touching or extending shared code (`internal/platform/`, `internal/domain/`, `extension/lib/shared/`). Name what you want to add and why, and what has to be de-hardcoded.
- Expect gentle pushback. Justify your claims well.

### Turn specs into plan
- Plan first: restate the task, list the files you'll touch, write a todo list.
- One feature or behaviour per change: small, reviewable diffs on a `feature/<name>` branch that lands on `develop` (see "Git Flow and CI"; `main` takes only releases).
- Every functionality gets its own file. Don't grow existing files into catch-alls.
- Add logical steps. Aim for small commits.

### Start to code!
- REFACTOR until the change fits into the existing code easily.
- TDD, strictly:
	- RED: write a failing test (and the Gherkin scenario in `server/spec/` for behaviour changes), run it, confirm it fails for the right reason. Bug fixes start with a test that reproduces the bug.
	- GREEN: write the minimum code to pass. Run the tests.
	- REFACTOR: clean up with tests green. Run the tests again.
		- Find and clean code smells (Refactoring, Martin Fowler).
		- KISS, YAGNI — unless your current plan needs it during this task.
- REFACTOR AGAIN!
	- Small functions with one purpose (aim for under ~30 lines).
	- Names explain intent without needing a comment.
	- No dead code, debug prints or leftover TODOs.
	- Prefer deleting code over adding it. Refactor duplication with tests green.
- Behaviour changes update `server/spec/`, the affected README and, for tool changes, `server/TOOLS.md`.

### Review checklist (run through before finishing)
- Does every new behaviour have a test that failed first, and a spec scenario?
- Is each file doing one thing?
- Any unrelated changes in the diff? Remove them or mention them.
- Are errors handled and wrapped with context?
- Any new dependency? It needs approval first.
- Tool changed? `server/TOOLS.md` regenerated, extension README and the agent guide `server/internal/guide/guide.md` still accurate.

## Git Flow and CI

### Branches
- `main`: released states only. The only way in is a pull request into `main` (the full CI suite runs on the merge result and is a required check): `release/*` → `main` (release finish), `hotfix/*` → `main` (hotfix finish) or `develop` → `main`. Never commit or push to it directly.
- `develop`: the integration branch. Finished work lands here.
- `feature/<name>`: one branch per task, branched from `develop`; put the slice or bead id in the name (`feature/E1-delivery-class`). Before landing: `git fetch && git rebase origin/develop`, run the local checks (below), then land it on `develop` in one of two ways: a pull request into `develop` (the reduced CI suite runs on the merge result) or a direct merge or push after the local checks (no CI runs). Bug fixes and docs changes are features too.
- `release/<x.y.z>`: branched from `develop` for a version jump (`<x.y.z>` = the planned release version); only stabilizing fixes go in (pull requests into the branch run the reduced suite). Release candidates are tagged on this branch (`v0.5.0-rc.1` on `release/0.5.0`; `release.yml` publishes them as pre-releases). Finish: pull request `release/<x.y.z>` → `main`, merge with a merge commit (no squash, no rebase), tag `v<x.y.z>` on the merge commit, merge back into `develop`. Steps: README "Releasing".
- `hotfix/<name>`: branched from `main` for an urgent fix; bump PATCH in `package.json`; pull request into `main` (full suite), tag `v<x.y.z>` on the merge commit, merge into `develop`.
- **Versions** follow Semantic Versioning 2.0.0: `MAJOR.MINOR.PATCH[-PRERELEASE]`. The `version` in `package.json` is the single source and carries the exact string being tagged; the tag is `v` plus it. A release from `develop` bumps MINOR (MAJOR for breaking changes; while MAJOR is 0, anything may change on MINOR; leaving 0.x is a later owner decision), a hotfix bumps PATCH. A release candidate has a pre-release suffix (`0.5.0-rc.1`, `0.5.0-prerelease01`; numeric identifiers without leading zeros): the release branch commits bump `package.json` once per tag (`0.5.0-rc.1`, `0.5.0-rc.2`, then `0.5.0`). No build metadata (`+build`: precedence ignores it, asset names suffer). A `v*` tag publishes a release, so `scripts/release_version.py` fails fast on any other form or on a tag that differs from `package.json`. A pre-release tag is published as a GitHub pre-release, so `releases/latest` (what `/bus server start` downloads) stays the last stable release. The wire `Protocol` integer (`server/internal/server/protocol.go`) and the extension's `MIN_SERVER_PROTOCOL` are a separate number; never confuse them with the version.

### CI
CI costs money. The local checks are the regular gate. Agents never trigger, poll or wait for a CI run (no `gh run watch`, no `gh pr checks --watch`): rely on the local checks and report them. CI runs on pull requests only, on the merge result, never on a branch push: a feature branch without a pull request and a direct merge into `develop` cost nothing.
- **Reduced suite** (`ci-*.yml`): a pull request into `develop`, `release/**` or `hotfix/**`. Build, unit tests and linters, no `-race`. One workflow per area, each run only when its paths change: `server/**` (go), `extension/**` and `tests/**` (typescript), `gui/**` and `server/internal/web/testdata/**` (web: GUI typecheck, tests, bundle build), `scripts/**` and `tests/test_*.py` (python). Never a required check (a skipped path-filtered job would block the merge).
- **Full suite** (`full.yml`): a pull request into `main`; never path-filtered, so its jobs `full-go`, `full-typescript`, `full-web`, `full-python`, `full-package` are the required checks of `main`. Everything: Go with `-race`, the TypeScript, GUI and Python suites, the real-bundle web build, the packaging build. Details, settings and cost: `docs/design/arch/pipeline.md` §2.
- Draft pull requests are skipped; a newer commit cancels the superseded run of the same pull request. `workflow_dispatch` is the manual escape hatch (for example the full suite on a release branch before a release candidate tag).
- Every workflow runs on `ubuntu-latest` only, never on Windows or macOS: we develop on Windows, so Windows and macOS are covered by local runs. To check for a Linux-only failure locally: `wsl -e bash -lc "cd /mnt/d/repos/<worktree>/server && go test ./..."` (WSL has go and gcc; its Go build cache is separate, so the first run is slow; add `-race` for the full-suite run).
- **Manual only**: none today (no suite needs a display or GPU).
- `release.yml` (tag `v*`) only publishes: strict SemVer check of the tag against `package.json`, build, GitHub release (`--prerelease` for a version with `-`). It repeats no suite and consults no earlier run: the full suite passed on the pull request into `main`.
- No third-party action beyond `actions/*`, `oven-sh/setup-bun` and `golangci/golangci-lint-action` without approval; `tests/workflows.test.ts` enforces the workflow rules.
- Session close: commit the feature branch including the tracked bead audit log (`.beads/interactions.jsonl` changes when a bead is closed), so the bead state lands with the code; land it on `develop` as above; then `bd dolt push` (the Dolt data under `refs/dolt/data` is separate from the git push and stays the last step of a session).

## Code conventions

### Go
- Accept interfaces, return structs. Keep interfaces small and define them at the consumer.
- Wrap errors with context: `fmt.Errorf("create space %s: %w", name, err)`. Never ignore an error. Return `apperr` kinds for client-visible failures.
- `ctx context.Context` is the first parameter of anything that does I/O.
- No package-level mutable state; pass dependencies explicitly. No `panic` for control flow.
- Table-driven tests, `t.Parallel()` where safe; no sleeping in tests (wait on channels or poll with a deadline).

### SQL
- Every schema change is a new numbered migration in `server/migrations/`. Never edit an applied one.
- `0001_schema.up.sql` is the one-time alpha reset: the whole schema, irreversible, no `.down.sql`. Every later migration must be reversible (`.down.sql`).
- A migration without `.down.sql` says so in its header line: `-- … Not reversible: …`.
- Later migrations get their number at merge time: the next free number; rename on rebase if another slice merged one first.
- Each migration runs in one transaction with foreign keys off and must pass `PRAGMA foreign_key_check` before it commits. Change a column definition by rebuilding the table the SQLite way (create `x_new`, copy, drop `x`, rename `x_new` to `x`, recreate its indexes); `ON DELETE` actions do not fire inside a migration.

### TypeScript
- `strict` mode, no `any`; use `unknown` and narrow. No untyped function signatures.
- Import with explicit `.ts` extensions.

### Python
- Type hints on all signatures; formatted and linted with ruff (`ruff.toml`: line length 120, Python 3.10+).

## Running it locally (smoke tests)
Facts that are not obvious from the code and that cost time to find out:

**Server**
- `agent-message-bus start` detaches the server and waits up to 10 s until `/healthz` answers with the instance id of the data dir's state file; on a timeout it prints the log tail and kills the server it spawned if that still holds the data dir and the state file names its pid. It is a no-op when a confirmed instance already serves that data dir, and fails after 10 s when the data dir's lock holder stays `state pending`; `stop` ends it.
- For a throwaway instance, always set all four: `AMB_ADDR=127.0.0.1:<port>`, `AMB_DB_PATH=<temp>/x.db`, `AMB_ADMIN_TOKEN=<32..512 visible ASCII chars>` and `AMB_WEB_ADDR=127.0.0.1:<port2>` or `off`. Without them the server uses the default data dir and ports: it touches your real DB, and a second instance fails to bind the web GUI's default `127.0.0.1:8081`. The log is written next to the db.
- The instance admin token authenticates as the bootstrap admin agent `admin` (kind admin), created on the first start; `bus_whoami` with it returns name `admin`. A dev DB that holds a leader or member named `admin` refuses to start: delete it.
- The HTTP API is `POST /v1/tools/<name>` with the JSON args as the body and `Authorization: Bearer <token>`. The result comes back as `{"result": …}`. RPC results may carry `"mail": {"waiting", "critical", "spaces"}`: once, on the first result after new mail reached an empty mailbox (or a critical entry arrived); `bus_inbox_read`, `bus_wait` or the SSE stream returning the entry counts as told, and a full `bus_inbox_ack` lets the next mail notify again. Strict parsers must allow the extra key.

**Loading the working-tree extension**
- Use `omp -e <repo>/extension/index.ts`. In omp 18.12, `--plugin-dir <repo>` did not register the extension's flags.
- A plugin installed with `omp plugin install` loads as well. Copies with the `agent-bus:instance` handshake (E10c) let only the first one to load run; an older installed copy without it (for example plugin 0.1.0) still registers its `bus_*` tools and `/bus` and opens its own inbox stream next to the working tree. Then smoke-test with `omp --no-extensions -e <repo>/extension/index.ts`, so only the working-tree copy loads.

**Env credentials**
- `AGENT_BUS_URL`/`AGENT_BUS_TOKEN` are read once at load and then deleted from `process.env`.
- With two copies loaded, only the first one to load sees them, so the working-tree copy may run without credentials and never activate its tools.
- For smoke tests, give the session a stored login instead: `OMP_AGENT_BUS_DIR=<temp>` with `credentials.json` containing `{"url":…,"token":…}`.

**omp flags and modes**
- omp rejects unknown flags with exit code 2. `--bus-url`/`--bus-accept` exist only while this extension is loaded, so a spawned worker needs it too: either installed and up to date, or forwarded `-e`.
- Print mode (`omp -p`) never starts a turn on inbox delivery, so an idle worker only wakes up in `tui`/`rpc`.

**omp tool lifecycle**
- omp fires `session_start` once per process. `/new`, `/resume` and `/fork` fire `session_switch`; `/branch` and `/btw` fire `session_branch`, and every branch moves to a new session file.
- Tools registered with `defaultInactive` stay off until the extension calls `setActiveTools`.
- A subagent gets the tools the extension registers for it: omp enables every tool an extension instance registers (`filterActiveTools` only drops parent-owned tools), so the extension registers an allow-list of the five blackboard tools for a subagent and withholds the rest. A subagent has no inbox stream of its own.

**Bus permissions to remember when writing test prompts**
- Only leaders and admins may send to `all` or send critical messages. A member that tries gets `forbidden`.
- A worker reports with `bus_send_direct` to the sender of its task (a leader or an admin); if that fails with `not_found`, it reports to a leader of the space. Results others need later go in a blackboard thread, and the report names its `threadId`.

## Generated files
`server/TOOLS.md` is generated from the tool registry and the templates `server/internal/tools/ledger_*.md.tmpl`; never edit it by hand. A golden test fails when it is stale:

```sh
cd server && go run ./cmd/tooldoc > TOOLS.md
```

The `/web` wire fixtures in `server/internal/web/testdata/` are marshalled from the real handlers; `gui/tests/api/contract.test.ts` decodes them with the GUI's decoders. Never edit them by hand; after a `/web` response changes, regenerate them (the Go test fails while they are stale):

```sh
cd server && go test ./internal/server -run TestWebContractFixtures -update
```

## Definition of ready
A slice is ready when (`docs/design/arch/definition-of-ready.md` §1):
- **Described**: goal, files, size, risks and dependencies in `docs/design/arch/slices.md` or the bead.
- **Specs approved**: the rules to move are named by file and line, their folder has no open REVIEW finding and no unresolved `@po-decision`; pure refactors state "no behaviour change".
- **No blocking question** open in `docs/design/arch/QUESTIONS.md`.
- **Contracts fixed**: every client-visible change is in `docs/design/arch/contracts.md`.
- **Dependencies merged**: every server slice it depends on is on `develop`.
- **Shared-code approval** for `internal/platform/**` or `internal/domain/**`; **no new dependency** without approval.
- **Acceptance tests named** with their RED reason.
- **Schema decided** in `docs/design/arch/schema.md` (or "—").
- **Docs listed**: TOOLS.md, tool-count sites, `spec/README.md`, `server/README.md`, `DEPLOYMENT.md`, `.omp/AGENTS.md` corrections.
- **Size ≤ M** (S ≈ ≤ 300 changed lines incl. tests, M ≈ ≤ 800).

## Definition of done
A slice is done when all hold:
1. **TDD evidence**: each acceptance test failed first for the stated reason (RED noted in the commit message or the bead) and passes now; bug fixes start with a reproducing test.
2. **Spec in sync**: the named rules are in `server/spec/*.feature` (verbatim scenario text), replaced lines are gone, `spec/README.md` features table/glossary updated. Only PO-decided scenarios are moved.
3. **Generated files**: tool changes → `server/TOOLS.md` regenerated, golden test green; the absolute tool count set at all sites.
4. **Contracts**: implementation matches `contracts.md`; deviations are updated there first; extension-facing change → `server/internal/server/protocol.go` bumped per policy.
5. **Schema**: new numbered migration with `.down.sql` (only `0001_schema` is irreversible), never an edit of a merged migration.
6. **Code rules**: one responsibility per file, functions ≲ 30 lines, errors wrapped with `%w`, `ctx` first for I/O, no global mutable state, no panic for control flow, no debug prints/dead code/TODOs. Features write only their own tables.
7. **Shared code** only as approved; no unapproved dependency.
8. **Docs**: README/DEPLOYMENT/extension README lines updated; the slice's `.omp/AGENTS.md` correction applied.
9. **Local checks** clean (below).
10. **Review**: self-review with the review checklist; no unrelated changes (or mentioned in the commit message); reviewer approval.
11. **Delivery**: branch `feature/<name>`, rebased on `origin/develop`, the local checks (below) clean, landed on `develop` by a pull request (reduced CI on the merge result) or by a direct merge after the local checks. The local checks are the gate; nobody waits for CI. `main` is reached only by a pull request from `release/*`, `hotfix/*` or `develop`.

Local checks, THE gate before a feature lands on `develop` (the reduced CI suite of a pull request re-runs only the areas it touches, on Linux, without `-race`):

```sh
cd server
gofmt -l .                     # prints nothing
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
go test -race ./...            # needs CGO; or: docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...
cd ..
bun test
bun run typecheck
(cd gui && bun test && bun run typecheck)
bun run build:gui && (cd server && go test ./internal/web/...)   # only when gui/ or server/internal/web/ changed: tests against the real bundle
pytest tests
uvx ruff@0.17.3 format --check . && uvx ruff@0.17.3 check .
```
