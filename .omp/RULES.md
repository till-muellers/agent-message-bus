# Hard rules

- Never write production code without a failing test first (Red -> Green -> Refactor). Run the test and confirm it fails for the right reason before implementing.
- One responsibility per file. Each server feature lives in its own folder under `server/internal/<feature>/` and writes only its own tables. No god files, no "utils" or "helpers" dumping ground.
- Keep functions small (under ~30 lines) and single-purpose.
- Go: never ignore an error, wrap with context (`%w`), pass `context.Context` first for I/O, no global mutable state, no `panic` for control flow.
- SQL: every schema change is a new numbered migration in `server/migrations/`. Never edit an applied one. Schema DDL never goes inline in Go strings; queries live in the feature's `repository.go`.
- Python and TypeScript: type hints on all signatures, strict TS, no `any`.
- No new dependency without asking first.
- Don't change code unrelated to the task. Mention it instead.
- No debug prints, dead code or leftover TODOs in the diff.
- Never hand-edit `server/TOOLS.md`; regenerate it with `go run ./cmd/tooldoc`.
- Never push to `main`: it is protected. Work on a branch and merge through a pull request with green CI.
- Before saying "done": run tests, lint and typecheck for every language you touched, and review your own diff.
- If requirements are unclear, ask before building.
