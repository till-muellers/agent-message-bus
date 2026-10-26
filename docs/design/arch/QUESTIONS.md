# Questions for the PO

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix are relative to `docs/design/specs/`.

Every question that was open after PO-1…PO-11, in one place: the architects' questions (Q-A*, formerly `definition-of-ready.md` §5), the client/pipeline questions (Q-P*, formerly `pipeline.md` §7) and the spec-analysis questions that `REVIEW.md:31` left open (Q-G* guidance, Q-O* omp-parity, Q-M* mail, Q-F* findings, Q-W* web; the number is the analyst's).

**Status (PO rulings, 2026-10-26):** Q-A1 was replaced by an own GUI listener; Q-M3 moves slice 14 to a future version; Q-A2…Q-A11 and Q-P1…Q-P3 are **decided as recommended**. No question blocks a slice. §4 (spec questions) keeps its kinds: confirm-default = implemented as recommended unless the PO says otherwise; info = no slice depends on it.

## 1. PO rulings that changed the design

| Id | Question | Ruling | Effect | Slices |
|---|---|---|---|---|
| Q-A1 | Approve `Config.Web` / `AMB_WEB` in `internal/platform/config` for "web GUI enabled explicitly" on a non-loopback address? | **Closed, replaced:** the GUI gets its own listen address `AMB_WEB_ADDR` (any `host:port`; empty or `off` disables it; default loopback on its own port, `127.0.0.1:8081` [AD-16]). The operator controls exposure by the address. The ruling is the `platform/config` shared-code approval. | W1a builds a second `http.Server`/listener in `internal/server`; graceful shutdown covers both; the API listener serves no `/web` (`contracts.md` §10, `slices.md` W1a). Spec-review rewrites `web/gui-access.feature`. | W1a (→ the web track, now unblocked) |
| Q-M3 | mail Q3 (`mail/analysis.md:318`): the stateful MCP inbox resource with subscriptions (M2): later or now? | **Decided: out of the current version.** | Slice 14 is `Deferred`, milestone `Future` (`BACKLOG.md`); not groomed now. | 14 |

## 2. Architecture (Q-A2…Q-A11): all decided as recommended (PO, 2026-10-26)

| Id | Question | Recommendation | Rationale | Affects |
|---|---|---|---|---|
| Q-A2 | Protocol starts at `2` and bumps at most once per release ([AD-1], `contracts.md` §6)? | Approve. | Matches the literal scenario `plugin-version-skew.feature:121-124` (a header-less server is legacy); one number per release keeps the extension's minimum meaningful. Arch-client aligned (CR-1). | 7, E9a |
| Q-A3 | `invites.invited_by TEXT NOT NULL DEFAULT ''` instead of nullable `TEXT` ([AD-2])? | Approve. | No NULL branch in Go; codes created before `0002` (a 10-minute window) still deliver their task, with `from ""` (`inbox.from_name` is NOT NULL). | 7 |
| Q-A4 | Slice-order deviations from REVIEW §11: 4 split into 4a–4d, 6 split with 6a before 4c, intermediate tool count 31? | Approve. | Each piece ≤ M, and every admin feature's Background needs `bus_mint_admin`, so 6a must come first to move scenarios verbatim. | 4a, 4b, 4c, 4d, 5, 6a, 6b |
| Q-A5 | = guidance Q10: `GET /v1/guide` bearer-protected (as specified in `agent-guide.feature:48-51`) rather than public like `/healthz`? | Bearer. | It is what the spec says; every `/v1` route needs a token, and the guide is only useful to an agent that has one. Making it public later is a one-line route change. | 11 |
| Q-A6 | = web Q10: web specs in a folder `server/spec/web/` ([AD-11])? | Yes. | Ten GUI feature files would crowd the flat `server/spec/`; a folder keeps the GUI transport apart from server behaviour. | W1a, W1b, W2, W3, 10 |
| Q-A7 | Dev-only `AMB_WEB_DIR` disk serving (Arch-client CR-G2): declined; re-open? | Keep declined. | A second `platform/config` switch for a dev-only need; no spec needs it; rebuild + restart is enough (`contracts.md` §12). Info only. | — |
| Q-A8 | Does the SSE stream count as "told" (it marks edge entries noticed, [AD-6]; `mail/analysis.md:149` "until a channel told the agent")? | Yes. | An agent with an open stream has received the entry; hinting again would duplicate it. | 13b |
| Q-A9 | Web sign-in rate limit as a local `429 rate_limited`, not a new `apperr` kind ([AD-9])? | Yes. | The limit exists only on the web sign-in; a global kind would leak into every transport and the MCP error mapping. | W1a |
| Q-A10 | The spec says the class is assigned "when it stores an entry" (`delivery-class.feature:39`); the design derives it from the stored `type` + `critical` ([AD-15]). Identical observably. OK? | Yes. | No migration and no drift; store it only when a future entry type needs a per-entry choice. | D1 |
| Q-A11 | Web mutation log-line key `actor` (admin, leader and member sessions) while the specs say "admin "ops"" (`gui-invite-cancel.feature:90`)? | `actor`. | Leader and member sessions sign in with their normal reach and may post or invite (`contracts.md` §10.1, REVIEW G1); `admin=` would be wrong for them. | W3, 10 |

## 3. Client and pipeline (Q-P1…Q-P3): all decided as recommended (PO, 2026-10-26)

| Id | Question | Recommendation | Rationale | Kind | Affects |
|---|---|---|---|---|---|
| Q-P1 | Self-contained Docker GUI build with an `oven/bun:1` build stage (new base image), or build the GUI before `docker build`? | Avoid the new image: build the GUI on the host first (variant A, below); the Dockerfile stays unchanged and PL-2 becomes docs only. | No image is published by any workflow, and the Dockerfile's `COPY . .` already carries a host-built `dist/` into the image, so the new dependency buys only "docker build without a local bun". | Decided (variant A) | PL-2 |
| Q-P2 | Make the CI `web` job a required status check on `main`? | Yes, after GUI-1 merges. | Otherwise a GUI break merges green; repo-settings change by the owner, no code. | Decided | GUI-1 |
| Q-P3 | Pin bun in CI (`bun-version`) to the version used locally? | Yes, to the version that wrote the first `gui/bun.lock`. | Avoids lockfile and bundler drift between local builds, CI and release. | Decided | GUI-1 |

### 3.1 Q-P1 evaluated: is the `oven/bun` stage avoidable?

Yes. Facts: `server/Dockerfile` builds with context `server/` and `COPY . .` (`:5`); `server/.dockerignore` excludes only `.git`, `*.db`, `data/`, so `server/internal/web/dist/` is copied when it exists; `release.yml` builds binaries with `scripts/build.py` and publishes no image; `ci.yml` has no Docker job and already uses `oven-sh/setup-bun@v2` (`ci.yml:44`), which GUI-1 also adds to `release.yml`.

| Variant | How | New dependency | Cost |
|---|---|---|---|
| **A (recommended)** | `bun run build:gui` on the host, then `docker build -t amb server` as today. | none | A user without bun gets an image that serves the fallback page ("The web GUI was not built; run the GUI build."). Documented in `DEPLOYMENT.md`/`README.md` by PL-2. |
| C (when images are published) | A workflow job runs `setup-bun` + `bun run build:gui`, then `docker build` with the `server/` context. | none (the action is already used in CI) | Only needed once the project publishes images; today it does not. |
| B | `FROM oven/bun:1 AS gui` stage, repo-root context, root `.dockerignore`. | `oven/bun:1` base image (approval needed) | Self-contained build; the build context and the documented command change. |

The PO only needs to answer if "`docker build` from a fresh checkout without bun shows the fallback page" is not acceptable; then B needs the approval and PL-2 grows to the Dockerfile change.

## 4. Spec questions REVIEW left open (`REVIEW.md:31`)

The architecture already assumes the analyst's default for each; answering confirms it or turns into a change of the named slice. Q-G10 = Q-A5 and Q-W10 = Q-A6 (§2).

| Id | Source | Question | Recommendation | Rationale | Kind | Affects |
|---|---|---|---|---|---|---|
| Q-G3 | `guidance/analysis.md:337` | Do members leave only when a leader says so (the leader revokes), or leave on their own after reporting? | Leave only when told; the leader revokes. | Already in the guide draft (`guide-draft.md:20`, `agent-guide.feature:80`); keeps the leader in control of hand-off; "no empty spaces" (5) covers the last departure. | Confirm-default | 11 |
| Q-G4 | `guidance/analysis.md:338` | Should leaders get `member_joined`/`member_left` inbox entries? | Not now. | New entry types are out of scope; presence (12) and the overview (8) show who is there. A later slice adds a type plus one `classByType` row. | Info | — |
| Q-G5 | `guidance/analysis.md:339` | Accept the `context`-event block, or also ask omp upstream for a persistent extension system-prompt section? | Accept the `context` block; no upstream ask needed now. | Works on omp 18.12 for every turn, including bus-started ones (`client.md` §5). | Confirm-default | E8b |
| Q-G7 | `guidance/analysis.md:341` | Fix a results-board convention and report cadence? | Report on done, blocked and decisions (no progress chatter); results go in a thread; no fixed board name. | Matches AGENTS X4 and the guide; a fixed board name is a convention the server cannot enforce. | Confirm-default | 11 |
| Q-G8 | `guidance/analysis.md:342` | Should `bus_space_overview` show pending invite names and unread counts; is 10 threads right? | No invites, no unread counts; 10 threads. | The overview stays a cheap read of space state; invites are visible to leaders in the GUI; per-caller unread counts cost a query per thread. | Confirm-default | 8 |
| Q-G9 | `guidance/analysis.md:343` | MCP `instructions`: static for every token, or per token with identity? | Static. | One text ≤ 4000 chars for every token, cacheable; identity comes from `bus_whoami` (`contracts.md` §7.2). | Confirm-default | 11 |
| Q-O3 | `omp-parity/analysis.md:326` | After the user pressed Esc, does bus mail restart the agent or wait for the user? | Wait for the user; a critical entry still steers the next turn. | Respects the user's interrupt; omp folds queued messages into the next turn (`client.md` §3.2). | Confirm-default | E1 |
| Q-O6 | `omp-parity/analysis.md:328` | Post-notice size (20 lines / 2 KiB) and coalescing per thread OK? | Yes. | Keeps a busy agent's context small; the full post is one `bus_read_thread` away. | Confirm-default | E10a |
| Q-O7 | `omp-parity/analysis.md:329` | Confirm we do not relay a woken agent's last output back to the sender? | Confirm: no auto-relay. | Avoids reply loops and token cost; agents reply explicitly with `bus_send_direct`. | Info | — |
| Q-O8 | `omp-parity/analysis.md:330` | Ask omp upstream for a public URL-scheme API (`read`/`write bus://…`)? | Not now. | Needs a public omp API that does not exist; the tools cover the need. | Info | — |
| Q-O12 | `omp-parity/analysis.md:334` | Is the `[via subagent <id>]` body prefix enough, or a structured `via` field on posts (migration)? | Prefix now. | No migration; the GUI shows the prefix in the body; a column can follow when a client needs to filter by it. | Confirm-default | E3 |
| Q-M5 | `mail/analysis.md:319` | `bus_wait` bounds 1..300 s, default 30 s? | Yes. | The default stays under common 60 s MCP tool timeouts; the cap bounds held connections (`contracts.md` §2.5). | Confirm-default | 9, E6 |
| Q-M7 | `mail/analysis.md:320` | "Empty" mailbox = no unacked entries (reading does not re-arm the notice)? | Yes. | Read-but-unacked mail is still waiting work; matches the notice spec and `schema.md` §2.2. | Confirm-default | 13a, 13b |
| Q-F1 | `findings/analysis.md:339` | One owner module for all three credential fields, or one owner per field? | One owner (option A). | One precedence table in one place; the per-field split keeps today's 11 write sites (`client.md` §2.1). | Confirm-default | E4a |
| Q-F4 | `findings/analysis.md:341` | Which launchers are supported (not only argv-tested)? | Tier-0 contract tests for all 9 now (P1b); tmux/kitty/zellij automated on Linux CI and the rest manual only when a later slice adds launcher CI. | P1b proves the argv contract without new CI infrastructure. | Info | P1b |
| Q-F6 | `findings/analysis.md:343` | May the contract wording in `launch/launcher.ts:22` be corrected to match `linux-terminal`'s behaviour? | Yes, in P1b. | P1b writes the contract tests for that wording; a comment-only change. | Confirm-default | P1b |
| Q-W3 | `web/analysis.md:589` | Loopback rule: none on the tool API, loopback-only for the GUI serving? | Superseded by the Q-A1 ruling: no loopback rule; the GUI has its own address `AMB_WEB_ADDR`, the tool API none. | The operator controls exposure by the address. | Decided (Q-A1) | W1a |
| Q-W5 | `web/analysis.md:590` | Admin inbox page: read-only, or also acknowledge? | Read-only. | The operator's omp session may stream the same inbox; a GUI ack would steal its entries (`contracts.md` §10.2). | Confirm-default | W1b, GUI-4 |
| Q-W6 | `web/analysis.md:591` | GUI live updates: polling or SSE? | Polling every 5 s. | No new broker topic family and no second stream slot; SSE can come later. | Confirm-default | W1b, GUI-3 |
| Q-W8 | `web/analysis.md:592` | Show when an admin last acted in `bus_list_admins`? | Not now. | Needs a migration and a write on every admin call; the audit log lines cover incidents. | Info | — |
| Q-W9 | `web/analysis.md:593` | GUI path and default: `/web`, enabled by default on loopback only? | `/web` on the GUI's own listener, default `127.0.0.1:8081` (Q-A1 ruling). | Keeps the cookie scoped (`Path=/web`, CR-11) and the GUI off public addresses unless the operator sets one. | Decided (Q-A1) | W1a |

## 5. Notes (no decision needed)

- `REVIEW.md` §12 still says "final registry count (35)"; the fixed fact at its top and `contracts.md` §1 say 34. `contracts.md` is authoritative.
- `REVIEW.md` §6/§11 give migrations fixed numbers (`0003_invites_id`, `0004_inbox_notice`) and make slice 10 wait for 13. The design instead assigns numbers at merge time (`schema.md` §2): names are fixed, numbers are the next free one, and 10 does not wait for 13a.
- Slice 0 copies the specs verbatim (only `tmp/…` paths are rewritten), so both notes stay visible in `docs/design/specs/REVIEW.md`.
