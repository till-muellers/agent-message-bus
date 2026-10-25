# Project guide — agent-message-bus

Conventions and working method for contributors and coding agents in this repo.

## Stack
- Server: Go (`server/`), SQLite embedded via pure-Go `modernc.org/sqlite`, MCP via `github.com/modelcontextprotocol/go-sdk`. One static binary, `CGO_ENABLED=0`.
- omp plugin: TypeScript on Bun (`extension/`, `skills/`), typechecked with `tsc --noEmit`.
- Python: release tooling only (`scripts/build.py`).
- CI: GitHub Actions (`.github/workflows/ci.yml`, `release.yml`).

## Layout

|Path|Contents|
|---|---|
|`server/`|Go server. `main.go`, `cmd/tooldoc/`, `internal/`, `migrations/`, `spec/`.|
|`extension/`|omp extension (TypeScript). `index.ts` wires `lib/*.ts`; `lib/shared/` holds helpers that are not tied to the bus.|
|`skills/agent-message-bus/`|Agent-facing skill.|
|`tests/`|Per-module `*.test.ts` (bun) with the shared `fakes.ts` and the launcher tests in `launch/`, plus `test_build.py` (pytest).|
|`scripts/build.py`|Release build: cross-compiles the server and writes `SHA256SUMS`.|
|`.omp/`|This guide and the hard rules (`RULES.md`).|

## Commands
- Go test: `cd server && go test ./...` (single pkg: `go test ./internal/<feature>/...`, single test: `go test -run TestName ./path`)
- Go race + cover: `go test -race -cover ./...` (needs CGO; CI runs it)
- Go lint/format: `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`
- TS: `bun test tests/`, `bun run typecheck`
- Python: `pytest tests` (format with `ruff format`)
- Tool ledger: `cd server && go run ./cmd/tooldoc > TOOLS.md`

## Architecture
- `server/main.go`: wiring and CLI dispatch only, no logic: config, CLI dispatch, DB, migrations, `server.Build`, `httpserver.Serve`.
- `server/internal/<feature>/`: one folder per feature (`auth`, `inbox`, `spaces`, `invites`, `messaging`, `blackboards`).
  - `service.go`: business logic
  - `repository.go`: all SQL of the feature (interfaces defined where they are used)
  - `*_test.go`: table-driven tests next to the code, real SQLite via `internal/platform/db/dbtest`
- **Features own their tables.** Each feature writes only its own tables. Cross-feature effects go through hooks (`spaces.CreationHook`, `spaces.DepartureHook`, `spaces.RevocationHook`, passed together in the `spaces.Hooks` parameter object) and service methods, inside the caller's transaction.
- Transports are thin: `internal/tools` (the handler layer: one registry of `bus_*` tools), `internal/rpc`, `internal/mcpserver`, `internal/admin`; `internal/server` composes everything (`Build`, routes).
- `internal/domain/`: business rules shared by features (names, body and title limits).
- `internal/platform/`: shared infra (apperr, db, broker, httpx, httpserver, config, instance, detach); only for things used by 2+ packages.
- `server/migrations/`: numbered SQL files; schema never inline in Go code.
- Dependencies point inward: transport → service → repository/platform. Never the reverse.
- Behaviour is specified in `server/spec/*.feature`; keep it in sync.

## Workflow
### Understand the specs
- Ask if requirements are unclear.
- Map the specs on the codebase. Try to answer these questions:
	- WHAT do I have to change or extend? Is this a new feature?
	- Is there an EXTENSION SEAM meant for my task (e.g. `spaces.CreationHook`/`DepartureHook`/`RevocationHook` via `spaces.Hooks`, the tool registry, `inbox.Deliver`)?
		- What pattern does it follow (strategy, hook, decorator, ...)?
		- Was it really made for tasks like mine? Is there an alternative?
		- Does it provide the required abstractions, or does it have to be expanded?
	- Is there code similar to my use case? What can I learn from it?
- Ask before introducing, touching or extending shared code (`internal/platform/`, `internal/domain/`, `extension/lib/shared/`). Name what you want to add and why, and what has to be de-hardcoded.
- Expect gentle pushback. Justify your claims well.

### Turn specs into plan
- Plan first: restate the task, list the files you'll touch, write a todo list.
- One feature or behaviour per change: small, reviewable diffs on a branch, merged by pull request (`main` is protected).
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
- Tool changed? `server/TOOLS.md` regenerated, extension README and skill still accurate.

## Code conventions

### Go
- Accept interfaces, return structs. Keep interfaces small and define them at the consumer.
- Wrap errors with context: `fmt.Errorf("create space %s: %w", name, err)`. Never ignore an error. Return `apperr` kinds for client-visible failures.
- `ctx context.Context` is the first parameter of anything that does I/O.
- No package-level mutable state; pass dependencies explicitly. No `panic` for control flow.
- Table-driven tests, `t.Parallel()` where safe; no sleeping in tests (wait on channels or poll with a deadline).

### SQL
- Every schema change is a new numbered migration in `server/migrations/`. Never edit an applied one.
- Migrations must be reversible (`.down.sql`), or explicitly marked as not.

### TypeScript
- `strict` mode, no `any`; use `unknown` and narrow. No untyped function signatures.
- Import with explicit `.ts` extensions.

### Python
- Type hints on all signatures, formatted with ruff.

## Running it locally (smoke tests)
Facts that are not obvious from the code and that cost time to find out:

**Server**
- `agent-message-bus start` detaches the server and returns at once; `stop` ends it.
- For a throwaway instance, set `AMB_ADDR=127.0.0.1:<port>`, `AMB_DB_PATH=<temp>/x.db` and `AMB_ADMIN_TOKEN=<32+ chars>`. The log is written next to the db.
- The HTTP API is `POST /v1/tools/<name>` with the JSON args as the body and `Authorization: Bearer <token>`. The result comes back as `{"result": …}`.

**Loading the working-tree extension**
- Use `omp -e <repo>/extension/index.ts`. In omp 18.12, `--plugin-dir <repo>` did not register the extension's flags.
- A plugin installed with `omp plugin install` loads as well. Both copies register the `bus_*` tools and the `/bus` command, and the installed copy may be an older version.

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
- Subagents only get the tools their parent passes on (`filterActiveTools` drops parent-owned tools), and they have no inbox stream of their own.

**Bus permissions to remember when writing test prompts**
- Only leaders may send to `all` or send critical messages. A member that tries gets `forbidden`.
- A worker should report with `bus_send_direct` to its leader or on a workstream topic.

## Generated files
`server/TOOLS.md` is generated from the tool registry and the templates `server/internal/tools/ledger_*.md.tmpl`; never edit it by hand. A golden test fails when it is stale:

```sh
cd server && go run ./cmd/tooldoc > TOOLS.md
```

## Definition of done
Tests pass, lint and typecheck are clean, the diff is reviewed, no unrelated changes, and the pull request's CI (`go`, `typescript`, `python`) is green. Locally:

```sh
cd server
gofmt -l .                     # prints nothing
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
go test -race ./...            # needs CGO; or: docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...
cd ..
bun test
bun run typecheck
pytest tests
```
