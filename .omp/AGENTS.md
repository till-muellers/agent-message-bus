# Project guide — agent-message-bus

Layout, feature ownership, Go/TypeScript conventions, generated files and the full definition-of-done commands are in the root [`AGENTS.md`](../AGENTS.md); omp loads it as well. This file adds the working method.

## Stack
- Server: Go (`server/`), SQLite embedded via pure-Go `modernc.org/sqlite`, MCP via `github.com/modelcontextprotocol/go-sdk`. One static binary, `CGO_ENABLED=0`.
- omp plugin: TypeScript on Bun (`extension/`, `skills/`), typechecked with `tsc --noEmit`.
- Python: release tooling only (`scripts/build.py`).
- CI: GitHub Actions (`.github/workflows/ci.yml`, `release.yml`).

## Commands
- Go test: `cd server && go test ./...` (single pkg: `go test ./internal/<feature>/...`, single test: `go test -run TestName ./path`)
- Go race + cover: `go test -race -cover ./...` (needs CGO; CI runs it)
- Go lint/format: `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`
- TS: `bun test tests/`, `bun run typecheck`
- Python: `pytest tests` (format with `ruff format`)
- Tool ledger: `cd server && go run ./cmd/tooldoc > TOOLS.md`

## Architecture
- `server/main.go`: wiring and CLI dispatch only, no logic.
- `server/internal/<feature>/`: one folder per feature (`auth`, `inbox`, `spaces`, `messaging`, `blackboards`).
  - `service.go`: business logic
  - `repository.go`: all SQL of the feature (interfaces defined where they are used)
  - `*_test.go`: table-driven tests next to the code, real SQLite via `internal/platform/db/dbtest`
- Transports are thin: `internal/tools` (the handler layer: one registry of `bus_*` tools), `internal/rpc`, `internal/mcpserver`, `internal/admin`; `internal/server` composes everything (`Build`, routes).
- `internal/domain/`: business rules shared by features (names, body and title limits).
- `internal/platform/`: shared infra (apperr, db, broker, httpx, httpserver, config, instance, detach); only for things used by 2+ packages.
- `server/migrations/`: numbered SQL files; schema never inline in Go code.
- Dependencies point inward: transport → service → repository/platform. Never the reverse. Features never write another feature's tables; use hooks or service methods.
- Behaviour is specified in `server/spec/*.feature`; keep it in sync.

## Workflow
### Understand the specs
- Ask if requirements are unclear.
- Map the specs on the codebase. Try to answer these questions:
	- WHAT do I have to change or extend? Is this a new feature?
	- Is there an EXTENSION SEAM meant for my task (e.g. `spaces.CreationHook`/`DepartureHook`, the tool registry, `inbox.Deliver`)?
		- What pattern does it follow (strategy, hook, decorator, ...)?
		- Was it really made for tasks like mine? Is there an alternative?
		- Does it provide the required abstractions, or does it have to be expanded?
	- Is there code similar to my use case? What can I learn from it?
- Ask before introducing, touching or extending shared code (`internal/platform/`, `internal/domain/`, `extension/lib/shared/`). Name what you want to add and why, and what has to be de-hardcoded.
- Expect gentle pushback. Justify your claims well.

### Turn specs into plan
- Plan first: restate the task, list the files you'll touch, write a todo list.
- One feature or behaviour per change: small, reviewable diffs on a branch, merged by pull request.
- Every functionality gets its own file. Don't grow existing files into catch-alls.
- Add logical steps. Aim for small commits.

### Start to code!
- REFACTOR until the change fits into the existing code easily.
- TDD, strictly:
	- RED: write a failing test (and the Gherkin scenario for behaviour changes), run it, confirm it fails for the right reason.
	- GREEN: write the minimum code to pass. Run the tests.
	- REFACTOR: clean up with tests green. Run the tests again.
		- Find and clean code smells (Refactoring, Martin Fowler).
		- KISS, YAGNI — unless your current plan needs it during this task.
- REFACTOR AGAIN!
	- Small functions with one purpose (aim for under ~30 lines).
	- Names explain intent without needing a comment.
	- No dead code, debug prints or leftover TODOs.
	- Prefer deleting code over adding it. Refactor duplication with tests green.

### Review checklist (run through before finishing)
- Does every new behaviour have a test that failed first, and a spec scenario?
- Is each file doing one thing?
- Any unrelated changes in the diff? Remove them or mention them.
- Are errors handled and wrapped with context?
- Any new dependency? It needs approval first.
- Tool changed? `server/TOOLS.md` regenerated, extension README and skill still accurate.

## Definition of done
Tests pass, lint and typecheck are clean (root `AGENTS.md` lists the commands), the diff is reviewed, no unrelated changes, and the pull request's CI (`go`, `typescript`, `python`) is green.

## Code conventions (in addition to the root AGENTS.md)

### Go
- Accept interfaces, return structs. Keep interfaces small and define them at the consumer.
- Never ignore an error. No `panic` for control flow.
- `t.Parallel()` where safe; no sleeping in tests (wait on channels or poll with a deadline).

### SQL
- Every schema change is a new numbered migration in `server/migrations/`. Never edit an applied one.
- Migrations must be reversible (`.down.sql`), or explicitly marked as not.

### Python / TypeScript
- Python: type hints on all signatures, formatted with ruff.
- TypeScript: strict mode, no `any`, no untyped function signatures.
