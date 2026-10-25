# AGENTS.md

Conventions for contributors and coding agents working in this repo.

## Layout

|Path|Contents|
|---|---|
|`server/`|Go server. `main.go`, `cmd/tooldoc/`, `internal/`, `migrations/`, `spec/`.|
|`extension/`|omp extension (TypeScript). `index.ts` wires `lib/*.ts`; `lib/shared/` holds helpers that are not tied to the bus.|
|`skills/agent-message-bus/`|Agent-facing skill.|
|`tests/`|`extension.test.ts` (bun) and `test_build.py` (pytest).|
|`scripts/build.py`|Release build: cross-compiles the server and writes `SHA256SUMS`.|

**Features own their tables.** Each feature in `server/internal/<feature>/` writes only its own tables. Cross-feature effects go through hooks (`spaces.CreationHook`, `spaces.DepartureHook`) and service methods, inside the caller's transaction.

## Workflow

- **TDD.** Write the failing test first (or the Gherkin scenario in `server/spec/` plus its Go test), then the code. Bug fixes start with a test that reproduces the bug.
- Behaviour changes update `server/spec/`, the affected README and, for tool changes, `server/TOOLS.md`.

## Go

- Wrap errors with context: `fmt.Errorf("create space %s: %w", name, err)`. Return `apperr` kinds for client-visible failures.
- `ctx context.Context` is the first parameter of anything that does I/O.
- No package-level mutable state; pass dependencies explicitly.
- Table-driven tests; real SQLite via `internal/platform/db/dbtest`.
- `main.go` is wiring only: config, CLI dispatch, DB, migrations, `server.Build`, `httpserver.Serve`.
- A feature is `internal/<feature>/` with `service.go` (logic) and `repository.go` (all SQL).
- `internal/platform/` is only for code with two or more users.
- Migrations in `server/migrations/` are never edited once applied; add a new numbered file.

## TypeScript

- `strict` mode, no `any`; use `unknown` and narrow.
- Import with explicit `.ts` extensions.

## Generated files

`server/TOOLS.md` is generated from the tool registry and the templates `server/internal/tools/ledger_*.md.tmpl`; never edit it by hand. A golden test fails when it is stale:

```sh
cd server && go run ./cmd/tooldoc > TOOLS.md
```

## Definition of done

All of these pass:

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
