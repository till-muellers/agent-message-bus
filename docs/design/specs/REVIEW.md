# Cross-folder spec review

Scope: `docs/design/specs/{guidance,omp-parity,mail,findings,web,admin}/`, checked against the final PO rulings, `server/spec/`, `server/migrations/` and `.omp/AGENTS.md`.

**Round 2 (resolution):** every finding was fixed in the spec files under `docs/design/specs/`, or it is listed below as waiting for a PO decision. No repo file was changed. `.omp/AGENTS.md` and `server/spec/` items are recorded as repo changes for the slice that implements them.

Status values: **Resolved** (the files that were edited are named), **PO** (a PO decision is pending; see the next section), **Slice N** (a repo change made in implementation slice N of §11), **OK** (no conflict).

Fixed facts used throughout:
- **Final tool registry: 34 server tools.** That is today's 29 plus `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`, `bus_space_overview` and `bus_wait`. There is no `bus_cancel_invite` (PO-3). The extension-local `bus_spawn_worker` comes on top. Each slice sets `transports.feature` to its step's count (§11).
- **Migrations:** only `0001_schema` has a fixed number (the reset). Later migrations have fixed names and take the next free number at merge time (`NNNN_<name>`, never folded into 0001): `NNNN_invites_invited_by` (slice 7), `NNNN_inbox_notice` (slice 13), `NNNN_invites_id` (slice 10). Slices do not wait for each other because of migration numbers. A stored delivery class, if the architects choose a column, is one more `NNNN_<name>`.

---

## PO decisions (all decided, round 6)

| # | Decision | Findings | Files edited |
|---|---|---|---|
| PO-1 | **Decided:** one server log line per GUI change (admin name, space, target, action). No table, no actions list. | B8 | `web/gui-mutations.feature` (log-line scenarios, header), `web/gui-invite-cancel.feature`, `web/analysis.md` §3.8, §4.1, §4.3, §6 |
| PO-2 | **Decided:** reads, sends, posts and thread creation `read`; management `write`; `bus_spawn_worker` `exec`. | C3 | `omp-parity/omp-tools.feature` (tag removed, `bus_cancel_invite` row removed), `omp-parity/analysis.md` Q2 |
| PO-3 | **Decided (changed):** cancelling an invite is **not a tool**; no `bus_cancel_invite`, so the registry is **34**. Cancelling is a GUI-only web API endpoint by an opaque invite id (numbered migration after 0001). `/bus spawn` exposes no cancel handle. | A5, B5, F4, G8 | New `web/gui-invite-cancel.feature`; `findings/invite-cancel.feature` is now a pointer; `web/invites.feature`, `web/gui-mutations.feature`, `web/analysis.md` §3.5, §6; `findings/analysis.md` item 6 and Q list; `omp-parity/omp-tools.feature`, `omp-subagents.feature`; counts in `guidance/`, `mail/`, `admin/` |
| PO-4 | **Decided:** the GUI deletes spaces behind a confirmation naming the space. | G2 | `web/gui-mutations.feature`, `web/gui-access.feature`, `web/analysis.md` §6 |
| PO-5 | **Decided + generalised:** lifecycle notices arrive on the next turn without waking the agent. Every inbox entry now carries a server-assigned **delivery class** `interrupt` \| `queue` \| `deferred` (from type and `critical`, part of the entry shape); clients map classes, not types. | D3 | New `mail/delivery-class.feature`; `omp-parity/omp-delivery.feature` (class mapping), `omp-parity/omp-blackboards.feature` (deferred rule), `omp-parity/analysis.md` Q5, `mail/analysis.md` file list |
| PO-6 | **Decided:** omp only; the upstream-pi `followUp` fallback rule is removed. | D8 | `omp-parity/omp-delivery.feature`, `omp-parity/analysis.md` Q10 |
| PO-7 | **Decided:** N1 + M11 with the critical re-notify. | F3, D7 | `mail/mail-notice.feature` (tag removed), `mail/analysis.md` §9 |
| PO-8 | **Decided:** S15 approved (map keyed by root session). | V3 | `omp-parity/analysis.md` Q13 |
| PO-9 | **Decided:** presence and receipts approved. | D6 | `omp-parity/analysis.md` Q9 |
| PO-10 | **Decided:** (b) embedded strict-TS SPA, bun build before `go build`, committed placeholder `dist/`. | W1–W3 | `web/analysis.md` §6 |
| PO-11 | **Decided:** S1, version check at spawn. | — | `findings/plugin-version-skew.feature`, `findings/analysis.md` Q list |

Other open questions in the analyses do not touch any finding here and are left as they are: guidance Q3, Q4, Q5, Q7–Q10; omp-parity Q3, Q6, Q7, Q8, Q12; mail Q3, Q5, Q7; findings Q1, Q4, Q6; web Q3, Q5, Q6, Q8–Q10.

---

## 1. Superseded `@admin` / "admin has no inbox" design

| ID | Sev | Finding | Status |
|---|---|---|---|
| A1 | H | `bus_wait` forbade admins | **Resolved:** `mail/bus-wait.feature` gets a rule "Admins wait on their own mailbox" (joined admin's wait ends on its mail; an unjoined space's `all` does not end it). |
| A2 | H | "The admin never gets a hint" | **Resolved:** `mail/mail-hint.feature` now has the admin's hint scenario. |
| A3 | H | No admin MCP inbox resource | **Resolved:** `mail/mcp-inbox-resource.feature`: an admin lists `bus://_inbox/<admin>`, and the URI table gains an admin row. |
| A4 | H | `mail/analysis.md` still specified the send-only `@admin` | **Resolved:** `mail/analysis.md` §0, §1, §3.1, §7 (now points at the adopted guide text), §8 (rewritten for named admins), §9 (Q9–Q12 superseded) and §12. |
| A5 | H | `bus_cancel_invite` forbade admins with the old message | **Resolved (PO-3):** the tool is gone; the GUI endpoint lets leaders of the space and admins cancel (`web/gui-invite-cancel.feature`). |
| A6 | M | "Pre-reset databases refused" | **Resolved:** `web/analysis.md` §2, §6; `web/admin-superuser.feature`. |
| A7 | L | Dangling "web §2.3" pointers | **Resolved:** `web/admin-superuser.feature`, `admin/analysis.md` (scope line and D6). |
| A8 | L | Stale "supersedes D11" clause | **Resolved:** `guidance/analysis.md` Q13. |
| A9 | L | Inbox descriptions kept "admin has no inbox" | **Resolved:** `mail/analysis.md` §7 (access "Leaders, members and admins"). |

## 2. Admin behaviour vs other folders

| ID | Sev | Finding | Status |
|---|---|---|---|
| B1 | H | Last-leader contradiction inside admin | **Resolved by the admin BA** (D15 narrowed: the refusal applies only while others remain). Aligned: `guidance/guide-draft.md` Leaders line, `guidance/agent-guide.feature` "wind down". |
| B2 | H | Web "revokes the last leader" scenario could not pass | **Resolved:** `web/gui-mutations.feature`. Now: `lead2` is revoked, then `lead`; `worker` remains and the space has no leader; plus a new scenario "revoking the last participant deletes the space". The "member of a space it is not in" premise was fixed as well. |
| B3 | M | `critical` described as "Leaders only" | **Resolved:** `guidance/tool-descriptions.feature` ("Leaders and admins only."), `omp-parity/omp-delivery.feature`, `admin/analysis.md` §5. `server/spec/README.md:59` → **Slice 4**. |
| B4 | M | May an admin spawn workers? | **Resolved (PO: yes):** `guidance/tool-descriptions.feature` (access "Leaders of the space and admins", task "from the caller", rule "An admin may spawn workers"), `guidance/session-context.feature` (admin toolset includes `bus_spawn_worker`), `guidance/guide-draft.md` Admins, `guidance/agent-guide.feature`, `guidance/analysis.md` Q13, `admin/analysis.md` §5. |
| B5 | M | Cancel rule named only leaders | **Resolved:** `findings/invite-cancel.feature`. A leader of another space now gets `not a participant of space demo`, as in every gate. |
| B6 | L | Overview description said "Participants only" | **Resolved:** `guidance/space-overview.feature`, `guidance/tool-descriptions.feature` (access rule). |
| B7 | M | Revoke text claimed a leader's tokens and boards are deleted | **Resolved:** `web/gui-mutations.feature` (confirm text depends on the role), `web/analysis.md` §3.8. |
| B8 | M | GUI audit beyond the ruled mint/revoke lines | **Resolved (PO-1):** one log line per GUI change; `web/gui-mutations.feature`, `web/analysis.md`. |
| B9 | L | Missing admin pitfalls on revoke and delete | **Resolved:** `guidance/tool-descriptions.feature`. |
| B10 | L | Subagent "never gets" list incomplete | **Resolved:** `omp-parity/omp-subagents.feature`. |

## 3. Tool names, parameters and counts

| ID | Sev | Finding | Status |
|---|---|---|---|
| C1 | M | Conflicting tool counts | **Resolved:** every folder states "final registry 34 server tools" (PO-3 removed `bus_cancel_invite`) and points to the step counts in §11. |
| C2 | M | omp discoverable list incomplete | **Resolved:** `omp-parity/omp-tools.feature` (all tools listed; later tools default to discoverable). |
| C3 | M | Approval tiers incomplete | **Resolved (PO-2):** `omp-parity/omp-tools.feature`. |
| C4 | OK | `bus_wait` contract consistent | — |
| C5 | OK | Subagent and essential tools consistent | — |
| C6 | L | omp-parity Q11 deviated from the five-tool ruling | **Resolved:** `omp-parity/analysis.md` (Q11 and R12 closed by the ruling; Q1 closed too). |
| C7 | L | `bus_wait`/inbox description rows missing | **Resolved:** `guidance/tool-descriptions.feature`; `mail/analysis.md` §7 points there. |
| C8 | L | `bus://` scheme collision | **Resolved:** the MCP resource uses `bus://_inbox/…` (`_` can never start a name). Edited `mail/mcp-inbox-resource.feature`, `mail/analysis.md`, `omp-parity/omp-bus-url.feature`. |

## 4. Ack and delivery semantics

| ID | Sev | Finding | Status |
|---|---|---|---|
| D1 | OK | Watermark agreed | — |
| D2 | M | Print/json idle entry: appended or unacked? | **Resolved:** it is queued as `nextTurn` and acked once a model request contains it (e.g. the one-shot run's first request). Edited: `omp-parity/omp-delivery.feature` (one-shot rule + new scenario), `omp-parity/analysis.md` S1, `mail/omp-mail-delivery.feature`, `mail/analysis.md` §1 and §4, `guidance/first-prompt.feature` (comment, bounded wait), `guidance/analysis.md`. |
| D3 | M | `nextTurn` entries stall the watermark | **Resolved (PO-5):** delivery class `deferred`; consumption and accepted stall in `omp-parity/omp-delivery.feature`; `mail/delivery-class.feature`. |
| D4 | M | Local `bus_wait` cannot retract asides | **Resolved:** `omp-parity/omp-tools.feature` ("queued" = held by the dispatcher; entries already handed to omp are not returned; new scenario). |
| D5 | M | Self-contradicting replay scenario | **Resolved:** `mail/omp-mail-delivery.feature` (retitled "is shown again"). |
| D6 | M | `bus_wait` vs `connected` | **Resolved (PO-9):** `omp-parity/presence.feature`. |
| D7 | M | GUI inbox read cleared the hint | **Resolved:** the GUI reads through a service listing that does not mark edges, and its proxy drops `mail`. Edited `web/gui-inbox.feature`, `web/analysis.md` §3.5, `mail/mail-hint.feature` (new rule), `mail/analysis.md` §8 and §12. The hint itself is PO-7. |
| D8 | L | Upstream-pi fallback | **Resolved (PO-6):** rule removed from `omp-parity/omp-delivery.feature`. |

## 5. Envelope, context block and subagents

| ID | Sev | Finding | Status |
|---|---|---|---|
| V1 | M | Guidance pinned the literal envelope text | **Resolved:** the steps assert type, sender and body; omp-rendering owns the text. Edited `guidance/first-prompt.feature`, `guidance/session-context.feature`, `guidance/guide-draft.md` (3854 chars, still ≤ 4000), `guidance/agent-guide.feature`, `guidance/analysis.md`. |
| V2 | M | omp-parity S13 on `before_agent_start` | **Resolved:** S13 dropped in `omp-parity/analysis.md` (the guidance `context` block is the roster). |
| V3 | M | Subagent scenarios need S15 | **Resolved (PO-8):** `@needs-S15` scenarios land after E4. |
| V4 | L | AGENTS.md reporting line | **Slice 11** (repo file; see X4). |

## 6. Migrations and schema

| ID | Sev | Change | Status |
|---|---|---|---|
| F1 | — | `0001_schema` (reset) | Slice 1. `admin/analysis.md` §7 also neutralises the `invites.task` comment and states that nothing else folds into 0001. |
| F2 | M | `invites.invited_by` | **Resolved:** `NNNN_invites_invited_by`, slice 7 (`guidance/analysis.md` change sketch). |
| F3 | M | Inbox edge/noticed columns | **Resolved (PO-7):** `NNNN_inbox_notice`, slice 13. |
| F4 | M | `invites.id` | **Resolved (PO-3):** `NNNN_invites_id`, slice 10 (GUI cancel handle). |
| F5 | L | Audit table, admin "last acted", posts `via` | Audit table dropped (PO-1). "Last acted" (web Q8) and `via` (omp-parity Q12) stay open; each would be a numbered migration after 0001. A stored delivery class, if the architects choose a column, likewise. |
| F6 | L | AGENTS.md migration rules vs reset | **Slice 1** (X5). |

## 7. `server/spec/` lines changed by several folders

All are repo changes, made by the slice named. The order in §11 removes the conflicts.

| ID | Sev | Spec lines | Status |
|---|---|---|---|
| S1 | M | `transports.feature` count, routes, admin rows | Slices 6, 8, 9 set the absolute count (32, 33, 34). Slice 7 adds the `Agent-Bus-Protocol` header (R2), slice 11 the guide route and instructions, slice 13 the `mail` field, W1/W3 the `/web` routes. |
| S2 | M | `spec/README.md:40` (followUp) | Slice E1. |
| S3 | M | `spec/README.md:49-51,59,95` glossary | Slice 4/6 (admin, critical), then slice 7 (invite task), then W3 (GUI invite cancel). |
| S4 | L | `spec/README.md:70-72,90` | Slices 5, 7, 10, W1; additive. |
| S5 | H | `invites.feature:8,34-42,57,61-64` | Slice 2 (`:34-42`), slice 7 (`:8,57,61-64`), slice 10 (invite id). `findings/invite-task-field.feature` fixed (R1). |
| S6 | H | `membership.feature` | Slices 4 and 5 (narrowed last-leader rule per the admin BA). |
| S7 | M | `inbox.feature` | Slice 4 rewrites the admin rows. Slices 9 and 13 no longer reintroduce "admin has no inbox" (A1–A3 fixed). |
| S8 | M | messaging/topics/blackboards/spaces admin rows; receipts | Slice 4; slice 12 adds receipts additively (admin scenarios assert "the call succeeds"). |
| S9 | L | `auth.feature`, `local-instance.feature` | Slices 4 and 6. |
| S10 | M | `ledger_*.md.tmpl` / `Tool.Access` | Slice 3 first, then admin access values in slices 4 and 6. |

## 8. Web

| ID | Sev | Finding | Status |
|---|---|---|---|
| G1 | M | Sign-in kinds vs "wrong-kind 401" | **Resolved:** `web/analysis.md` §4.3 (leaders and members may sign in; 401 only for unknown, malformed or non-Bearer). |
| G2 | M | Milestone comment vs delete-space | **Resolved (PO-4):** `web/gui-access.feature`, `web/gui-mutations.feature`. |
| G3 | L | "never creates, deletes" | **Resolved:** `web/spaces-and-participants.feature`. |
| G4 | M | "never lists inbox of any agent" | **Resolved:** `web/topics.feature` ("any other agent"). |
| G5 | L | Impossible premise in gui-inbox | **Resolved:** `web/gui-inbox.feature`. |
| G6 | L | "published npm plugin", `skills` | **Resolved:** `web/analysis.md` §3.6. |
| G7 | M | Duplicate read model | **Resolved:** `/web/api/state` composes `internal/overview` + `invites.ListPending` (`web/analysis.md` §3.5, `web/live-updates.feature`). |
| G8 | L | No GUI invite cancel | **Resolved (PO-3):** `web/gui-invite-cancel.feature`; `web/invites.feature` lists the id. |

## 9. Findings / release

| ID | Sev | Finding | Status |
|---|---|---|---|
| R1 | M | Change list edited the retired SKILL.md | **Resolved:** `findings/invite-task-field.feature` header and task wording, `findings/analysis.md` item 3. |
| R2 | M | A new extension on an old server loses the task | **Resolved:** the server reports `Agent-Bus-Protocol: N` on `/healthz` (body stays `ok`). The extension checks it before redeeming a code or activating tools and refuses a too-old server. Edited `findings/plugin-version-skew.feature` (new rule), `findings/analysis.md` item 8 R6, `guidance/first-prompt.feature`, `guidance/analysis.md`. The header lands in slice 7 and the extension check in E9. |
| R3 | L | `package.json` touched by skill removal and version bump | **Slice P2** (same release PR as E8). |

## 10. `.omp/AGENTS.md` (repo file; not edited this round)

| ID | Sev | Line | Fix | Slice |
|---|---|---|---|---|
| X1 | H | 130 | Subagents get what their own extension instance registers; withhold at registration | E3 |
| X2 | M | 7, 17, 84 | Drop `skills/`; the checklist names the guide | E8 |
| X3 | M | 133 | "Only leaders and admins may send to `all` or send critical" | 4 |
| X4 | L | 134 | Report to the task's sender, fall back to a leader, put results in a thread | 11 |
| X5 | M | 96-97 | One-time reset exception; "irreversible" header convention | 1 |
| X6 | L | 32, 37 | Add `internal/overview`, `internal/guide`, `internal/web` | 8, 11, W1 |
| X7 | L | 111-112 | The instance token is the bootstrap admin `admin`; results may carry `mail` | 6, 13 |

---

## 11. Implementation order (PR slices)

Each server slice moves its `.feature` files into `server/spec/` and sets the absolute tool count.

| # | Slice | Spec files | Depends on | Tools after | Migration |
|---|---|---|---|---|---|
| 1 | **Schema reset** (+ AGENTS X5, release-note line) | `admin/schema-reset.feature` | — | 29 | `0001_schema` |
| 2 | **`domain.ValidText`** + task error | `findings/invite-task-field.feature` | — | 29 | — |
| 3 | **`Tool.Access` + description fixes** (non-admin rows), `bus_spawn_worker` in `TOOLS.md` | `guidance/tool-descriptions.feature` (non-admin rules) | — | 29 | — |
| 4 | **Admin core**: bootstrap principal, seam B, resolver, inbox, lifecycle, subscriptions, revoke-leader; admin access lines incl. `critical` (+ AGENTS X3) | `admin/admin-role.feature`, `admin/admin-messaging.feature` | 1, 3 | 29 | — |
| 5 | **No empty spaces** (narrowed last-leader rule) | `admin/no-empty-spaces.feature` | 4 | 29 | — |
| 6 | **Admin tools**, `amb_admin_`, audit lines, bootstrap-only token swap | `admin/admin-tokens.feature` | 4, 5 | **32** | — |
| 7 | **Invite task as direct message** (server) + `Agent-Bus-Protocol` header on `/healthz` | `guidance/first-prompt.feature` (server rules), `findings/plugin-version-skew.feature` (server-protocol scenario) | 1, 2, 4 | 32 | `NNNN_invites_invited_by` |
| 8 | **`bus_space_overview`** | `guidance/space-overview.feature` | 4 | **33** | — |
| 9 | **Server `bus_wait`** (admins included) | `mail/bus-wait.feature` | 4 | **34** | — |
| 9b | **Delivery class** on every inbox entry (`interrupt` \| `queue` \| `deferred`), server-assigned (PO-5) | `mail/delivery-class.feature` | 4 | 34 | only if stored as a column (next free number) |
| 10 | **Invite id** + `invites.ListPending` (no tool; GUI handle, PO-3) | `web/gui-invite-cancel.feature` (listing rules), `web/invites.feature` | 4, 7 | 34 | `NNNN_invites_id` |
| 11 | **Guide**: `internal/guide`, MCP `instructions`, `GET /v1/guide`, `TOOLS.md` section (+ AGENTS X4) | `guidance/agent-guide.feature`, `guide-draft.md` | 3, 6, 8, 9 | 34 | — |
| 12 | **Presence/receipts** (PO-9) | `omp-parity/presence.feature` | 4, 9 | 34 | — |
| 13 | **Mail notice N1 + hint M11** (PO-7) | `mail/mail-notice.feature`, `mail/mail-hint.feature` | 9 | 34 | `NNNN_inbox_notice` |
| 14 | MCP inbox resource — **later, a future version** (PO Q-M3: not in the current version) | `mail/mcp-inbox-resource.feature` | 13 | 34 | — |
| E1 | **omp delivery by class** (interrupt→steer, queue→aside, deferred→nextTurn; one-shot idle queue→nextTurn), docs (`spec/README.md:40`, `ledger_tail.md.tmpl:37`) | `omp-parity/omp-delivery.feature` | 9b | — | — |
| E2 | **Ack watermark** | `mail/omp-mail-delivery.feature`, `omp-delivery.feature` ack rule | E1 | — | — |
| E3 | **Subagent allow-list + attribution** (+ AGENTS X1) | `omp-parity/omp-subagents.feature` rules 1, 2, 4 (+ stop-gap R11) | — | — | — |
| E4 | Findings items 1 and 4 (credential owner, `createSession` move), then **S15** (PO-8) | findings items 1, 4; `omp-subagents.feature` rule 3 | E3 | — | — |
| E5 | **Envelope + renderer** | `omp-parity/omp-rendering.feature` | E1 | — | — |
| E6 | **Local `bus_wait` shadow** + dispatcher | `omp-parity/omp-tools.feature` `bus_wait` rule, `mail/omp-mail-delivery.feature` last rules | 9, E2 | — | — |
| E7 | **loadMode / approval / no `bus_inbox_ack`** (PO-2) | `omp-parity/omp-tools.feature` rules 1–2 | E6 | — | — |
| E8 | **Context block + admin-token sessions (spawn included) + skill retirement** (+ AGENTS X2) | `guidance/session-context.feature` (`@needs-S15` scenarios after E4), guidance §6 cutover | 6, 8, 11, E3, E4 | — | — |
| E9 | **Invite task, extension side** + server-protocol check (refuse a too-old server) | `guidance/first-prompt.feature` (extension rules), `findings/plugin-version-skew.feature` (extension-side scenarios of the server-protocol rule) | 7, E1, E2 | — | — |
| E10 | Blackboard notices, `pi.events`, receipt texts | `omp-blackboards.feature`, `omp-events.feature`, `omp-tools.feature` receipts | E1, E2, 12 | — | — |
| W1 | **Web GUI milestone 1** (PO-10): session, state endpoint (overview + `ListPending`), own inbox read-only | `web/gui-access`, `spaces-and-participants`, `topics`, `blackboards`, `invites`, `live-updates`, `gui-inbox` | 6, 8, 10 | — | — |
| W2 | **Web admin area** | `web/gui-admins.feature` | 6, W1 | — | — |
| W3 | **Web mutations** incl. delete space (PO-4), invite cancel by id (PO-3), one log line per change (PO-1) | `web/gui-mutations.feature`, `web/gui-invite-cancel.feature` | 5, 10, W1 | — | — |
| P1 | Findings item 2 (`lib/spawn/`), item 7 Tier-0 launcher tests | findings items 2, 7 | before E9 | — | — |
| P2 | **Release**: version bump and tag check (findings R1–R5, S1 per PO-11), plugin and server in one release with the matching protocol, "delete pre-release DBs" note, `package.json` `files` | `findings/plugin-version-skew.feature` | all slices of that release | — | — |

Critical path: 1 → 4 → 5 → 6 → (7, 8, 9) → 11 → E8. The extension track E1 → E2 → E6 → E7 runs alongside. E9 needs 7, so the release that changes the invite task also contains E9's protocol check.

## 12. Summary

- **Resolved in the specs:** every finding except the repo-side items (AGENTS.md X1–X7, `server/spec/` S1–S10), which are assigned to slices; all PO decisions PO-1..PO-11 are applied (round 6).
- **Folder state:** mail and findings are aligned with the admin role (admins have mailboxes, `bus_wait`, hints and the inbox resource; invite cancel is a GUI-only endpoint). Every folder states one final registry count (34) and the migration rule (only `0001_schema` is numbered; `NNNN_invites_invited_by`, `NNNN_inbox_notice`, `NNNN_invites_id` take the next free number at merge time).
- **Admins may spawn workers**, and the specs say so. Old server + new extension is now refused through `Agent-Bus-Protocol` on `/healthz` (`findings/plugin-version-skew.feature`).
- **Repo-side items** (AGENTS.md X1–X7, `server/spec/` S1–S10) are assigned to the slices in §11.
