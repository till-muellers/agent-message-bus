# Definition of Ready, Definition of Done, readiness

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix (e.g. `admin/analysis.md`) are relative to `docs/design/specs/`.

Applies to every server slice/bead in [`slices.md`](slices.md) (one slice = one bead = one branch = one PR). Arch-client adds a "DoR review (client)" section for E*/GUI/P* slices.

## 1. Definition of Ready (all must be checked before a branch is opened)

| # | Criterion | How to check |
|---|---|---|
| R1 | **Slice is described**: goal, files, size, risks, dependencies in `slices.md` (or the bead body). | entry exists with all fields |
| R2 | **Spec scenarios exist and are approved.** The rules to move are named by file and line in the committed design (`docs/design/specs/`, slice 0); their folder has no open finding in `docs/design/specs/REVIEW.md` (status Resolved/OK) and no unresolved `@po-decision` tag on a moved scenario. Pure refactors state "no behaviour change; existing scenarios stay green" instead. | grep the moved rules for `@po-decision`; REVIEW status |
| R3 | **No blocking question open**: every question in `QUESTIONS.md` whose "Blocks" column names the slice is answered. "Confirm-default" questions do not block: the slice implements the recommendation, and a different answer arrives as a contract change. All PO-1…PO-11 are decided. | `QUESTIONS.md` |
| R4 | **Contracts fixed**: every client-visible change (tool params/results, errors, HTTP shapes, headers, entries) is in `contracts.md`, and Arch-client has acknowledged it. | contracts.md section named in the slice |
| R5 | **Dependencies merged**: every server slice it depends on is merged to `main`. Client slices are never prerequisites of a server slice; contract changes the client needs are covered by R4. | `git log main` |
| R6 | **Shared-code approval** obtained for any change in `internal/platform/**` or `internal/domain/**` (AGENTS.md:54). | approval recorded (PR link or PO note) |
| R7 | **No new dependency**, or its approval is recorded (RULES.md:9). | `go.mod` unchanged |
| R8 | **Acceptance tests named**: the Go test names (table-driven, real SQLite via `dbtest`) and the RED reason ("fails because …") are listed. | slice "Tests" line |
| R9 | **Schema decided**: migration name, DDL and down file in `schema.md`, or "—". The number is not part of readiness: it is the next free number at merge time (`schema.md` §2), renumbered on rebase. | schema.md section |
| R10 | **Docs listed**: TOOLS.md regeneration, tool-count sites, `spec/README.md`, `server/README.md`, `DEPLOYMENT.md`, the `.omp/AGENTS.md` correction(s) assigned to the slice. | slice "Docs" line |
| R11 | **Size ≤ M** (S ≈ ≤ 300 changed lines incl. tests, M ≈ ≤ 800). L is split before it is ready. | slice "Size" |

## 2. Definition of Done (reconciles `.omp/AGENTS.md` "Definition of done", lines 143-156, with its workflow and review checklist, lines 63-84)

A slice is done when **all** hold:

1. **TDD evidence**: each acceptance test from R8 failed first for the stated reason (RED noted in the PR description) and passes now; bug fixes start with a reproducing test (AGENTS.md:65-68).
2. **Spec in sync**: the named rules are in `server/spec/*.feature` (verbatim scenario text), replaced lines are gone, `spec/README.md` features table/glossary updated (AGENTS.md:42,76). Only PO-decided scenarios are moved.
3. **Generated files**: tool changes → `server/TOOLS.md` regenerated (`go run ./cmd/tooldoc > TOOLS.md`), golden test green; the absolute tool count set at all five sites (`slices.md` header).
4. **Contracts**: implementation matches `contracts.md`; any deviation updated there first and sent to Arch-client; extension-facing change → `server/internal/server/protocol.go` bumped per policy (`contracts.md` §6).
5. **Schema**: new numbered migration with `.down.sql` (only `0001_schema` is irreversible), never an edit of a merged migration (AGENTS.md:96-97 after X5).
6. **Code rules**: one responsibility per file, functions ≲ 30 lines, errors wrapped with `%w`, `ctx` first for I/O, no global mutable state, no panic for control flow, no debug prints/dead code/TODOs (RULES.md; AGENTS.md:72-75,86-93). Features write only their own tables; cross-feature effects via hooks/service methods in the caller's tx.
7. **Shared code** only as approved (R6); no unapproved dependency (R7).
8. **Docs**: README/DEPLOYMENT/extension README lines named in R10 updated; the slice's `.omp/AGENTS.md` correction applied.
9. **Local checks** clean (AGENTS.md:146-156): `gofmt -l .` prints nothing; `go vet ./...`; `golangci-lint run ./...`; `go test -race ./...`; `bun test`; `bun run typecheck`; `pytest tests`.
10. **Review**: self-review with the AGENTS.md:78-84 checklist; no unrelated changes (or mentioned in the PR); reviewer approval.
11. **Delivery**: branch `slice/<id>-<slug>`, PR to protected `main`, CI `go`, `typescript`, `python` green, merged by PR.

Proposed amendment to `.omp/AGENTS.md` (X9, slice 1): replace lines 143-156 with §2 items 1–11 (keeping the command block) and add §1 as "Definition of ready".

## 3. Readiness now (2026-10-26, `main` = `526635d`, after the PO decisions on PO-1…PO-11 and the lead's reconciliation)

Per-slice status for every server and client slice is in `BACKLOG.md` (authoritative); this section explains the server part.

### Ready

| Slice | Why it passes the DoR |
|---|---|
| **0** Commit the design | docs only; no deps; makes R2/R4 checkable on `main` |

### Ready as soon as slice 0 merges

| Slice | Why it passes the DoR |
|---|---|
| **1** Schema reset | admin folder questions all closed (`admin/analysis.md:309`); DDL in `schema.md` §1; no shared code (runner unchanged); tests named; S |
| **2** `ValidText` + task error | shared-code change approved (`findings/analysis.md:127-131`); S |
| **3** `Tool.Access` + descriptions | no PO tag in `tool-descriptions.feature`; interim and final access sentences fixed (`contracts.md` §3.1); M |
| **4b** Membership seam B | refactor, no behaviour change; S |
| **D1** Delivery class | PO-5 decided; spec `mail/delivery-class.feature` exists (no `@po-decision`); contract `contracts.md` §4.1, CR-12 accepted; no migration (AD-15, Q-A10 confirm-default); S. Moves the invite-task scenario only after 7 |

### Blocked only by unmerged dependencies (ready as soon as these merge)

| Slice | Waits for |
|---|---|
| 4a Bootstrap admin | 1 |
| 6a Mint/list admins | 3, 4a |
| 4c Admin authority | 3, 4b, 6a |
| 4d Admin participation | 4c |
| 5 No empty spaces | 4d |
| 6b Revoke admins | 5, 6a |
| 7 Task as DM + protocol | 1, 2, 4c (Q-A2, Q-A3 confirm-default) |
| 8 `bus_space_overview` | 3, 4d |
| 9 Server `bus_wait` | 3, 4d |
| 11 Guide | 3, 6b, 8, 9 (bearer `GET /v1/guide` as specified; Q-A5 confirm-default) |
| 12 Presence/receipts (PO-9 approved) | 4d, 9 |
| 13a Mail notice (PO-7 approved) | 4d, 9 |
| 13b Mail hint (PO-7 approved) | 13a (Q-A8 confirm-default) |
| W1b Web reads | W1a, 7, 8 |
| W2 Web admin area | W1a, 6b |
| W3 Web mutations (PO-1, PO-4 decided) | 5, W1b, W2 (Q-A11 confirm-default) |
| 10 Web invite cancel by id (PO-3 decided; spec `web/gui-invite-cancel.feature`) | 4c, 7, W1b, W2 |

### Deferred (PO ruling)

| Slice | Reason | Deps |
|---|---|---|
| 14 MCP inbox resource | out of the current version (PO ruling on Q-M3); size L → split before any future pickup | 13b |

W1a (Web sessions) is now blocked only by its dependencies 4a, 6b: the PO replaced Q-A1 with the own GUI listener `AMB_WEB_ADDR`, and that ruling is the `platform/config` shared-code approval (R6).

No pending PO decision blocks a server slice. PO-2, PO-6, PO-8, PO-11 are client-only; PO-5 created D1.

## 4. `.omp/AGENTS.md` corrections

See `slices.md` (last table): X1 → E3, X2 → E8c, X3 → 4c, X4 → 11, X5 → 1, X6 → 8/11/W1a, X7 → 4a (bootstrap admin) and 13b (`mail`), X8 (new: `AMB_WEB_ADDR` smoke line) → W1a, X9 (new: DoR/DoD) → 1, X10 (new: `docs/design/` layout row) → 0.

## 5. Questions needing an answer

Moved to `QUESTIONS.md`, together with the client questions (Q-P1…Q-P3) and the spec questions REVIEW left open. All architecture and pipeline questions are decided (PO, 2026-10-26); no question blocks a slice.
