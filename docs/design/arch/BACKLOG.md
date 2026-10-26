# Backlog — every slice, server and client

> **Committed in slice 0.** Spec paths in the table are relative to `docs/design/specs/`; contract sections are `docs/design/arch/contracts.md` §n.

Input for issue-tracker grooming: one row per slice, ids exactly as in `slices.md` (server) and `slices-client.md` (client). The slice files hold the full description (goal, files, tests, docs, risks, smoke); this table holds what a tracker needs. Status as of 2026-10-26 (`main` = `526635d`, nothing merged yet).

**Columns**
- `order`: suggested global merge order; every `depends_on` id appears earlier.
- `track`: `design`; `server` (server slices outside `/web`); `server-web` (`/web` backend); `ext-structure`, `ext-delivery`, `ext-release` (`slices-client.md` tracks 1–3); `gui-pipeline` (track 4).
- `size`: S ≈ ≤ 300 changed lines incl. tests, M ≈ ≤ 800 (DoR R11); `L-split` must be split before it is ready.
- `depends_on`: hard prerequisites, comma-separated ids (server and client ids never collide); `—` = none. Soft ordering preferences are in the notes below.
- `dor_status`: `Ready` | `Blocked-by-deps` (only unmerged `depends_on`) | `Blocked-by-question` (a blocking question in `QUESTIONS.md`, plus any deps) | `Deferred` (out of the current version by PO ruling; not groomed).
- `blocking_q` / `default_q`: question ids from `QUESTIONS.md`. All questions are now decided (PO, 2026-10-26); `default_q` lists the decided questions whose answer the slice implements.
- `milestone`: `M0` design; `M1` protocol-2 release = everything merged before the first release tag after slice 7 (server 1–7, D1, 8, 9, 11, 12, 13a, 13b; client P1a, P1b, E4a, E9a, E1, E2, E9b; closed by P2b); `M2` extension release (the remaining extension slices; still protocol 2, so no `MIN_SERVER_PROTOCOL` bump, `contracts.md` §6); `M3` web GUI; `Future` (not in the current version).
- `specs`: feature files (or analysis items) the slice implements; `contracts`: `contracts.md` sections it implements or consumes.

| order | id | title | track | size | depends_on | dor_status | blocking_q | default_q | milestone | specs | contracts |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 0 | Commit the design to docs/design | design | S | — | Ready | — | — | M0 | — (copies all of `tmp/specs`, `tmp/arch`) | — |
| 2 | D1 | Delivery class on every inbox entry | server | S | 0 | Blocked-by-deps | — | Q-A10 | M1 | mail/delivery-class.feature | §4, §4.1, §4.2, §12 |
| 3 | 1 | Schema reset | server | S | 0 | Blocked-by-deps | — | — | M1 | admin/schema-reset.feature | — |
| 4 | 2 | domain.ValidText + invite task error | server | S | 0 | Blocked-by-deps | — | — | M1 | findings/invite-task-field.feature | §3.3, §11 |
| 5 | 3 | Tool.Access + purpose-first descriptions | server | M | 0 | Blocked-by-deps | — | — | M1 | guidance/tool-descriptions.feature | §3.1 |
| 6 | 4b | Membership seam B (refactor) | server | S | 0 | Blocked-by-deps | — | — | M1 | — (no behaviour change) | — |
| 7 | P1a | Layout: feature folders | ext-structure | M | 0 | Blocked-by-deps | — | — | M1 | findings/analysis.md (item 2) | — |
| 8 | 4a | Bootstrap admin identity | server | M | 1 | Blocked-by-deps | — | Q-A4 | M1 | admin/admin-tokens.feature, admin/schema-reset.feature | §3.2, §8, §11 |
| 9 | 6a | Mint and list admins; bootstrap-only token swap | server | M | 3, 4a | Blocked-by-deps | — | Q-A4 | M1 | admin/admin-tokens.feature | §1, §2.1, §2.2, §8, §11 |
| 10 | 4c | Admin authority in every space | server | M | 3, 4b, 6a | Blocked-by-deps | — | Q-A4 | M1 | admin/admin-role.feature, admin/admin-messaging.feature, guidance/tool-descriptions.feature | §3.1, §11 |
| 11 | 4d | Admin participation, lifecycle and inbox | server | M | 4c | Blocked-by-deps | — | Q-A4 | M1 | admin/admin-role.feature, admin/admin-messaging.feature, guidance/tool-descriptions.feature | §3.1, §3.2, §4.2, §11 |
| 12 | 5 | No empty spaces | server | S | 4d | Blocked-by-deps | — | Q-A4 | M1 | admin/no-empty-spaces.feature | §11 |
| 13 | 6b | Revoke admins | server | M | 5, 6a | Blocked-by-deps | — | Q-A4 | M1 | admin/admin-tokens.feature, admin/no-empty-spaces.feature | §1, §2.3, §11 |
| 14 | 7 | Invite task as direct message + Agent-Bus-Protocol | server | M | 1, 2, 4c | Blocked-by-deps | — | Q-A2, Q-A3 | M1 | guidance/first-prompt.feature, findings/plugin-version-skew.feature | §3.3, §4.2, §5, §6 |
| 15 | E9a | Server protocol gate + protocol-sync fitness | ext-delivery | S | 7, D1, P1a | Blocked-by-deps | — | Q-A2 | M1 | findings/plugin-version-skew.feature | §6, §12 |
| 16 | E1 | Delivery class mapping: steer / aside / nextTurn | ext-delivery | S | D1, E9a, P1a | Blocked-by-deps | — | Q-O3 | M1 | omp-parity/omp-delivery.feature, mail/delivery-class.feature, omp-parity/omp-blackboards.feature | §4.1 |
| 17 | E2 | Ack watermark on consumption | ext-delivery | M | E1 | Blocked-by-deps | — | — | M1 | mail/omp-mail-delivery.feature, omp-parity/omp-delivery.feature | §4.2 |
| 18 | E9b | Invite task as a direct message (extension side) | ext-delivery | S | 7, E1, E2, E9a | Blocked-by-deps | — | — | M1 | guidance/first-prompt.feature | §4.2, §5 |
| 19 | P2b | Release: version bump + tag check | ext-release | S | 1, 2, 3, 4a, 4b, 4c, 4d, 5, 6a, 6b, 7, D1, E9a, E1, E2, E9b | Blocked-by-deps | — | — | M1 | findings/plugin-version-skew.feature | §6 |
| 20 | P1b | Tier-0 launcher tests | ext-structure | S | P1a | Blocked-by-deps | — | Q-F4, Q-F6 | M1 | findings/analysis.md (item 7) | — |
| 21 | E4a | One credential owner + createSession move | ext-structure | M | P1a | Blocked-by-deps | — | Q-F1 | M1 | findings/analysis.md (items 1, 4) | — |
| 22 | P2a | Worker plugin skew check (S1) | ext-release | M | P1a | Blocked-by-deps | — | — | M2 | findings/plugin-version-skew.feature | — |
| 23 | E3 | Subagent allow-list, local tools in main only, attribution | ext-delivery | M | E4a | Blocked-by-deps | — | Q-O12 | M2 | omp-parity/omp-subagents.feature, guidance/session-context.feature, omp-parity/omp-delivery.feature, mail/omp-mail-delivery.feature | §4.2 |
| 24 | E4b | Subagent identity map (S15) | ext-delivery | S | E3 | Blocked-by-deps | — | — | M2 | omp-parity/omp-subagents.feature, guidance/session-context.feature | — |
| 25 | E8a | Admin-token sessions | ext-delivery | S | 4a, 4b, 4c, 4d, 6a, E4a | Blocked-by-deps | — | — | M2 | guidance/session-context.feature | §3.2, §4.2 |
| 26 | E5a | Envelope, details, escaping | ext-delivery | S | E1, E2 | Blocked-by-deps | — | — | M2 | omp-parity/omp-rendering.feature | §4 |
| 27 | E5b | Transcript card renderer | ext-delivery | S | E5a | Blocked-by-deps | — | — | M2 | omp-parity/omp-rendering.feature | — |
| 28 | E10c | pi.events + two-copy handshake | ext-delivery | S | E2 | Blocked-by-deps | — | — | M2 | omp-parity/omp-events.feature | — |
| 29 | 8 | bus_space_overview | server | M | 3, 4d | Blocked-by-deps | — | Q-G8 | M1 | guidance/space-overview.feature, guidance/tool-descriptions.feature | §1, §2.4 |
| 30 | 9 | Server bus_wait | server | M | 3, 4d | Blocked-by-deps | — | Q-M5 | M1 | mail/bus-wait.feature, guidance/tool-descriptions.feature | §1, §2.5, §11, §12 |
| 31 | 12 | Presence and receipts | server | M | 4d, 9 | Blocked-by-deps | — | — | M1 | omp-parity/presence.feature | §3.4 |
| 32 | 13a | Mail notice (edge) | server | M | 4d, 9 | Blocked-by-deps | — | Q-M7 | M1 | mail/mail-notice.feature | §9 |
| 33 | 13b | "You have mail" hint | server | M | 13a | Blocked-by-deps | — | Q-A8, Q-M7 | M1 | mail/mail-hint.feature | §0, §4.2, §9, §12 |
| 34 | 11 | Guide | server | M | 3, 6b, 8, 9 | Blocked-by-deps | — | Q-A5, Q-G3, Q-G7, Q-G9 | M1 | guidance/agent-guide.feature, guidance/guide-draft.md | §7 |
| 35 | E6 | Local bus_wait shadow + dispatcher hold | ext-delivery | M | 9, E2 | Blocked-by-deps | — | Q-M5 | M2 | omp-parity/omp-tools.feature, mail/omp-mail-delivery.feature, omp-parity/omp-delivery.feature | §2.5, §6, §12 |
| 36 | E7 | Tool presentation + no bus_inbox_ack | ext-delivery | S | E6 | Blocked-by-deps | — | — | M2 | omp-parity/omp-tools.feature, mail/mail-hint.feature, guidance/session-context.feature, guidance/tool-descriptions.feature | §2.5, §3.1, §9 |
| 37 | E10a | Blackboard notices | ext-delivery | M | E2, E5a, E6 | Blocked-by-deps | — | Q-O6 | M2 | omp-parity/omp-blackboards.feature | §4.1 |
| 38 | E10b | Status line + thread document rendering | ext-delivery | S | E10a | Blocked-by-deps | — | — | M2 | omp-parity/omp-blackboards.feature | — |
| 39 | E10d | Receipt texts + outgoing card | ext-delivery | S | 12, P1a | Blocked-by-deps | — | — | M2 | omp-parity/omp-tools.feature | §3.4 |
| 40 | E8b | Context block | ext-delivery | M | 4d, 11, E2, E3, E4b, E8a | Blocked-by-deps | — | Q-G5 | M2 | guidance/session-context.feature | §3.2, §7.1 |
| 41 | E8c | Skill retirement | ext-delivery | S | 11, E8b | Blocked-by-deps | — | — | M2 | guidance/analysis.md (§6 cutover), guidance/agent-guide.feature | — |
| 42 | W1a | Web transport, own listener (`AMB_WEB_ADDR`) and sessions | server-web | M | 4a, 6b | Blocked-by-deps | — | Q-A1, Q-A6, Q-A9, Q-W3, Q-W9 | M3 | web/gui-access.feature | §10, §10.1, §10.5, §11, §12 |
| 43 | GUI-1 | Scaffold + build into the embed dir + CI web job | gui-pipeline | M | W1a | Blocked-by-deps | — | Q-P2, Q-P3 | M3 | web/gui-access.feature | §10.5 |
| 44 | GUI-2 | Shell: session, router, store, mount, login, layout | gui-pipeline | M | GUI-1, W1a | Blocked-by-deps | — | — | M3 | web/gui-access.feature, web/spaces-and-participants.feature | §10.1 |
| 45 | PL-2 | Docker image with the GUI | gui-pipeline | S | GUI-1 | Blocked-by-deps | — | Q-P1 | M3 | — (packaging) | §10.5 |
| 46 | W1b | Web reads | server-web | M | W1a, 7, 8 | Blocked-by-deps | — | Q-A6, Q-W5, Q-W6 | M3 | web/spaces-and-participants.feature, web/topics.feature, web/blackboards.feature, web/invites.feature, web/live-updates.feature, web/gui-inbox.feature | §10.2 |
| 47 | W2 | Web admin area | server-web | S | W1a, 6b | Blocked-by-deps | — | Q-A6 | M3 | web/gui-admins.feature | §10.3, §11 |
| 48 | GUI-3 | Space list + space view + polling | gui-pipeline | M | W1b, GUI-2 | Blocked-by-deps | — | Q-W6 | M3 | web/spaces-and-participants.feature, web/topics.feature, web/invites.feature, web/live-updates.feature, web/blackboards.feature | §10.2 |
| 49 | GUI-4 | Board, thread and inbox views | gui-pipeline | S | W1b, GUI-3 | Blocked-by-deps | — | Q-W5 | M3 | web/blackboards.feature, web/gui-inbox.feature | §10.2 |
| 50 | GUI-5 | Admin area | gui-pipeline | S | W2, GUI-2 | Blocked-by-deps | — | — | M3 | web/gui-admins.feature | §10.3 |
| 51 | W3 | Web mutations | server-web | M | 5, W1b, W2 | Blocked-by-deps | — | Q-A6, Q-A11 | M3 | web/gui-mutations.feature | §10.4, §11 |
| 52 | 10 | Web invite cancel by id | server-web | M | 4c, 7, W1b, W2 | Blocked-by-deps | — | Q-A6, Q-A11 | M3 | web/gui-invite-cancel.feature | §10.2, §10.6, §11 |
| 53 | GUI-6 | Mutations and invite cancel | gui-pipeline | M | W3, 10, GUI-3, GUI-4 | Blocked-by-deps | — | — | M3 | web/gui-mutations.feature, web/gui-invite-cancel.feature | §10.4, §10.6 |
| 54 | 14 | MCP inbox resource (future version; split before pickup) | server | L-split | 13b | Deferred | — | Q-M3 | Future | mail/mcp-inbox-resource.feature | — (not yet specified) |

**Totals:** 54 slices (25 server incl. 0 and 14, 29 client). Ready: 1 (`0`). Deferred: 1 (`14`, future version, Q-M3). Blocked-by-question: 0. Blocked-by-deps: 52; once `0` merges, `D1`, `1`, `2`, `3`, `4b` and `P1a` are Ready.

## Notes for grooming

- **Critical path:** 0 → 1 → 4a → 6a → 4c → 4d → 5 → 6b → W1a → W1b → W3 → GUI-6. The extension path to the protocol-2 release is 0 → P1a → E9a (needs 7, D1) → E1 → E2 → E9b → P2b.
- **The web track** (W1a and the 11 slices behind it) is gated only by its dependencies 4a and 6b; the GUI listens on its own address `AMB_WEB_ADDR` (PO ruling on Q-A1).
- **Migration numbers are assigned at merge time** (`schema.md` §2): 7, 13a and 10 each add a named migration and take the next free number when they merge, so they may merge in any order; there is no ordering edge between them.
- **Same-release pairs:** 7 with E9a + E9b; E1 with E2 (an aside dropped between them is lost); D1 before E1. No tag was cut after slice 7, so 8, 9, 12, 13a, 13b (and 11 if it merges before the tag) ship under protocol 2: `MIN_SERVER_PROTOCOL` stays 2 and E6/E8b need no protocol-3 bump (`contracts.md` §6). Only an extension-facing change merged after the tag bumps to 3.
- **P2b** is built once (first run closes M1) and then runs at every release; its later runs are not new slices.
- **Merge early:** 3 touches every tool file; open 6a/8/9/10 branches after it merges (`slices.md` slice 3).
- **Slice 0** is the one R11 size exception (a verbatim copy of reviewed files).
