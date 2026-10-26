# Admin as a full role: analysis

Scope: the PO's new admin ruling. It replaces the "admin token is the superuser `@admin`" design (`docs/design/specs/web/admin-superuser.feature`, now marked superseded, and the round-3 revision of `docs/design/specs/web/analysis.md` §2). The participant-row inventory and the sqlite rebuild probes of that round are re-rated in §4.1 below.

No production code, spec or doc was changed. Deliverables:

| File | Covers |
| :--- | :--- |
| [`admin-role.feature`](admin-role.feature) | Admin authority: a leader in every space without joining; reads, sends, posts, closes, removes, invites, revokes members, creates and deletes spaces |
| [`admin-messaging.feature`](admin-messaging.feature) | Joining (participant role `admin`), being messaged, `all` and subscriptions, the admin's own inbox and stream slot, leaving |
| [`admin-tokens.feature`](admin-tokens.feature) | Bootstrap admin `admin`, `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`, rotation, admin endpoints |
| [`schema-reset.feature`](schema-reset.feature) | One fresh schema `0001_schema.up.sql` as the first iteration; runner unchanged; no detection of old databases |
| [`no-empty-spaces.feature`](no-empty-spaces.feature) | Invariant: the last participant's departure (leave, revoke, `bus_revoke_admin`) deletes the space |

Also updated, as asked: `docs/design/specs/web/admin-superuser.feature` (now only a superseded header) and the admin Rule of `docs/design/specs/mail/mail-notice.feature`.

---

## 1. Current-state map

### 1.1 Identity and authentication

| Fact | Where |
| :--- | :--- |
| Principal kinds `admin`, `leader`, `member`; `Principal{Kind, AgentID, AgentName, SpaceID}` | `server/internal/auth/principal.go:12-24` |
| The admin token is one in-memory SHA-256 hash (`atomic.Pointer`), compared in constant time **before** any DB lookup. The match returns `Principal{Kind: Admin}` with **no agent id and no name**. | `server/internal/auth/service.go:16-28,31-35` |
| Leader/member tokens resolve through `tokens JOIN agents`; the kind comes from `agents.kind` | `server/internal/auth/repository.go:13-26` |
| Live swap of the admin hash (≥ 32 chars) | `server/internal/auth/admin_token.go:7-18` |
| `MintLeader` requires `p.Kind == Admin` ("admin token required"), validates the name and reuses the agent: re-mint gives an additional token | `server/internal/auth/service.go:47-67` |
| One name, one kind: an existing agent of another kind yields `conflict "agent %s is a %s agent"` | `server/internal/auth/service.go:104-113` |
| Minted tokens are `amb_` + 43 base64url characters; the instance token is `amb_admin_` + 43 | `server/internal/auth/token.go:11-18`, `server/internal/platform/config/token.go:48-55` |
| The instance token is `AMB_ADMIN_TOKEN`, else `<datadir>/admin-token` (0600, generated on first use) | `server/internal/platform/config/token.go:19-39` |
| `run`: resolve the token → `db.Open` → `db.Migrate` → `serve` → `server.Build(…, token, …)` | `server/main.go:112-129,131-147` |
| `server.Build` builds `auth.NewService(conn, adminToken, time.Now)` and wires every service | `server/internal/server/build.go:21-37` |
| CLI `rotate-token`: new token, `POST /v1/admin/admin-token` to the live instance with the old token, then the file is written. Refused when `AMB_ADMIN_TOKEN` is set. | `server/internal/platform/instance/token.go:22-58` |
| CLI `stop` uses the instance token for `POST /v1/admin/shutdown` and kills on fallback | `server/internal/platform/instance/stop.go:19-26` |
| CLI `start` prints the log tail when the server does not become healthy | `server/internal/platform/instance/start.go:32-35` |
| MCP user id is `kind:agentID`; that is `admin:0` for the admin today | `server/internal/mcpserver/verifier.go:26-29` |

### 1.2 Admin endpoints and tools

| Fact | Where |
| :--- | :--- |
| `POST /v1/admin/shutdown`, `POST /v1/admin/admin-token` behind `authn(guard(…))` | `server/internal/admin/handler.go:20-25`, `server/internal/server/routes.go:30` |
| `guard`: loopback first ("admin endpoints are only available from localhost"), then `p.Kind == Admin` ("admin token required") | `server/internal/admin/guard.go:13-35` |
| Token handler decodes `{"token"}` and calls `SetAdminToken` | `server/internal/admin/handler.go:34-49` |
| The only admin tool is `bus_mint_leader` | `server/internal/tools/admin.go:13-27` |
| Every other route (`/v1/tools`, `/mcp`, `/v1/inbox/stream`) is bearer-only, with no network check | `server/internal/server/routes.go:24-29` |

### 1.3 Where the admin is refused today

| Gate | Message | Where |
| :--- | :--- | :--- |
| `requireAgent` in the resolver: every space-scoped call | "admin token cannot act in spaces; mint a leader with bus_mint_leader" | `server/internal/spaces/resolver.go:19-25`, called at `:54` |
| `List` spaces | same | `server/internal/spaces/service.go:62` |
| `requireLeaderPrincipal`: create and join a space | "only leaders may create spaces" / "join spaces" | `server/internal/spaces/resolver.go:27-33`; `service.go:104,142` |
| Inbox read, ack and stream | "admin token has no inbox; mint a leader with bus_mint_leader" | `server/internal/inbox/service.go:74-104`; `inbox/handler.go:46-52` |
| `Whoami` returns early with no name and `spaces []` | — | `server/internal/spaces/whoami.go:24-28` |

### 1.4 Authority and participation inside a space

- `ResolveTx` is the single gate: it looks up the space, applies the member token's space binding and the participant row → `Membership{SpaceID, SpaceName, AgentID, AgentName, Role}` (`server/internal/spaces/resolver.go:53-75`, `membership.go:18-25`).
- **Leader-authority checks** test `m.Role != RoleLeader` at six places: `messaging/service.go:57` (critical), `messaging/topics.go:79` (`all`), `messaging/topics.go:124` (close a topic), `blackboards/boards.go:55` (delete a board), `blackboards/threads.go:108` (remove a thread), `spaces/service.go:95-100` (`requireLeaderRole`: invite, revoke, delete space).
- **Participation-role checks** test the row's role: `spaces/service.go:189` (last leader cannot leave), `spaces/revoke.go:75` (leaders cannot be revoked), `blackboards/departure.go:17` (only members' boards go), `spaces/service.go:80-93` (`summarize` counts leaders, everyone else counts as a member).
- **Auto-subscriptions** of the creator: `messaging/topics.go:52` (topic), `blackboards/threads.go:76` (thread). **Subscribe/unsubscribe**: `messaging/topics.go:101-112`, `blackboards/posts.go:39-56`.
- **Join** uses `ResolveTx` as an "am I a participant?" probe: success means conflict, `forbidden` means insert (`spaces/service.go:141-159`). An admin that resolves without a row would break that probe.
- **Delete** counts the other leaders (`spaces/service.go:171-177`, SQL `spaces/repository.go:120-121`). **Leave** removes the row through `removeParticipant`: departure hooks, inbox rows of that space, space tokens, row (`spaces/service.go:183-219`).
- **Recipients:** `all` = `participantIDs` (`spaces/repository.go:167-168`); direct = `participantByName` (`messaging/direct.go:22-28`); the sender name is `m.AgentName` (`spaces/membership.go:55-63`).
- **Participant order:** leaders first, then everyone else by name (`spaces/repository.go:145-146`).

### 1.5 Schema and migrations

| Fact | Where |
| :--- | :--- |
| `agents.kind CHECK (kind IN ('leader','member'))` | `server/migrations/0001_init.up.sql:1-2` |
| `participants.role CHECK (role IN ('leader','member'))` | `server/migrations/0001_init.up.sql:9-12` |
| `tokens.space_id` NULL = leader token | `server/migrations/0001_init.up.sql:5-8` |
| `invites` table | `server/migrations/0002_invites.up.sql:1-9` |
| Runner: applies `^\d{4}_.+\.up\.sql$` in order, each in one `InTx`, recorded in `schema_migrations`. **`.down.sql` files are never read.** | `server/internal/platform/db/migrate.go:14,22-81` |
| `//go:embed *.sql` | `server/migrations/embed.go:6-9` |
| DSN: `foreign_keys(1)`, WAL, busy timeout; one connection | `server/internal/platform/db/db.go:15-29` |
| Test DBs run the same migrations; fixtures seed agents with **explicit ids starting at 1** | `server/internal/platform/db/dbtest/dbtest.go:23`, `dbtest/seed.go:22-25`, e.g. `invites/fixtures_test.go:38`, `messaging/fixtures_test.go:34-41` |

### 1.6 Consumers that encode the old admin

- Specs: `server/spec/auth.feature:6-12,38-40,73-102`, `spaces.feature:17-20,82,125,156-158,181-190,228`, `membership.feature:51,93-95,150-152`, `inbox.feature:108-110,148-150,259-262`, `direct-messages.feature:92-94`, `topics.feature:339-341`, `blackboards.feature:28`, `transports.feature:35,54-57,232-246`, `local-instance.feature:79-107`, `spec/README.md:42,49-50,89,95`.
- Docs: `server/README.md:23-24,32,48,64-67,86,106`, `server/DEPLOYMENT.md:38,48,56,66,205,207,219-222`, root `README.md:48,60-61`, `server/TOOLS.md` (generated).
- Extension: an admin `/bus login` stores `adminToken` and activates nothing (`extension/lib/command.ts:63-66`, `extension/README.md:26,29-32,57,74`). `/bus promote` mints a session leader (`command.ts:76-84`).

---

## 2. Rulings and the decisions derived from them

PO rulings (authoritative): **R1** admin is a full role with its own inbox, uses the admin tools and is a leader in every space without joining. **R2** messaging an admin needs a joined space. **R3** the bootstrap token comes from startup, and admins mint further admins. **R4** one-time schema reset. **R5** the goal is a full administration website.

Derived decisions (each is specified in the features):

| # | Decision | Why |
| :--- | :--- | :--- |
| D1 | Admins are **named agents**: one `agents` row of kind `admin` per admin, each with its own inbox and stream slot | R1 demands an inbox. Several people or consoles must not share one stream slot (one identity per agent, `server/spec/README.md:88`). Attribution and revocation need one identity per admin. See §4.2 |
| D2 | The **bootstrap admin is the agent `admin`**, ensured (created or reused) at every server start. The instance token (env or file) authenticates as it. | `admin` is a legal name, so it can be addressed by `bus_send_direct` once joined (R2). `@admin` could never be addressed, because recipient names are agent names. |
| D3 | The **instance token stays in memory** (not in `tokens`). `admin-token` and `rotate-token` work as today and change only the bootstrap admin's credential. | One source of truth (env/file). The CLI (`stop`, `rotate-token`) needs it even with an empty DB. See §4.2b |
| D4 | **The bootstrap admin cannot be revoked**, and the bootstrap name cannot be minted | So there is always ≥ 1 admin and no "last admin" rule is needed. A DB token for `admin` would survive rotation. |
| D5 | Any admin mints admins (`bus_mint_admin`), lists them (`bus_list_admins`) and revokes **minted** admins, itself included (`bus_revoke_admin`) | R3. You cannot revoke what you cannot see. Self-revoke is harmless because the bootstrap admin remains. |
| D6 | Revoking an admin deletes its tokens and its participations (departure path) and **keeps the agent row** | `posts.author_id`, `threads.created_by`, `topics.created_by`, `blackboards.owner_id` reference `agents(id)` without cascade (sqlite probe of the superseded web round, re-rated in §4.1). Re-minting the name revives the same identity. |
| D7 | **Authority ≠ participation.** Every admin leads every space. A participant row with the new role **`admin`** exists only where it joined or created the space. | R1 + R2. A `leader` row would distort leader counts, "last leader" and Delete (§4.1). |
| D8 | An admin that has not joined **cannot subscribe** and is **not auto-subscribed** to topics or threads it creates | Keeps the invariant "an agent's inbox only holds entries of spaces it participates in", which `removeParticipant` relies on for cleanup (R2). |
| D9 | Admins may **create** spaces (they join as `admin`) and **join** and **leave** spaces with the existing tools | Leader parity for creation. Joining is how R2 is reached. |
| D10 | **Delete space: any space** (PO confirmed). | A non-participant admin would always see "other leaders", so strict parity would make the power vacuous. |
| D11 | **Admins are a strict superset of leaders** (PO ruling). They revoke members **and leaders** in every space, the last leader included. Nobody revokes an admin from a space with `bus_revoke_member`; admins use `bus_revoke_admin`. | Revoking a leader removes its participation only: departure hooks, its inbox entries in that space. Its leader tokens are not space-bound and stay valid. Its boards stay (role ≠ member). **Edge case, last leader:** the space survives with its members and no leader row. Admins still lead it, any leader may `bus_join_space` it, and the "last leader cannot leave" rule is untouched, because it guards self-departure only. |
| D12 | `POST /v1/admin/admin-token` (replace the instance token) is **bootstrap-only** (PO confirmed). Shutdown is open to every admin. Both stay loopback-only. | A minted admin that replaces the instance token would desync `<datadir>/admin-token` and lock the CLI out. |
| D13 | **Schema reset as a first iteration** (PO ruling): one `0001_schema.up.sql`, old files deleted, runner unchanged, no stamp and no detection. Pre-reset DBs are deleted by hand (release-notes/README line). | §4.3, §7 |
| D14 | **One `slog.Info` audit line per admin mint and revoke** (PO accepted): `msg "admin minted"` / `"admin revoked"` with `by`, `name` (and `deleted` for revoke); never the token; refused calls log nothing. | §6.2 |
| D15 | **No empty spaces** (PO ruling Q6: delete on leave). After `removeParticipant` deletes the row, it counts the remaining participants in the same transaction. At 0 it calls the existing `deleteSpace` (`spaces/repository.go:128`); the FK cascades clean up topics, subscriptions, boards, threads, posts, invites, member tokens and inbox rows. That one place covers leave, `bus_revoke_member` (members, and leaders by an admin) and `bus_revoke_admin` (per joined space). Entries produced by departure hooks for a deleted space are dropped, because no recipient remains. | **The last-leader rule is narrowed:** "last leader cannot leave; delete the space instead" applies only while other participants remain (`membership.feature:137-140` has a member present, so it is unchanged). A lone leader may leave, and the space is deleted. Implementation: `Leave` runs `countOtherLeaders` only if `countParticipants > 1`. Leaderless spaces still exist (an admin creates one, or revokes the last leader while others remain); admins lead them. Revoking only an invite code removes no participant. |
| D16 | **Minted admin tokens are `amb_admin_` + 43 base64url** (PO ruling Q9; leader/member tokens stay `amb_`). | `Authenticate` (`auth/service.go:31-44`) compares the presented token's hash with the in-memory instance hash first; a minted token never matches it and falls through to `lookupToken` (`auth/repository.go:13-26`), which takes the kind from `agents.kind`. The prefix is never parsed, so it grants nothing: an unknown `amb_admin_…` string is `unauthenticated "invalid token"`. Change: `NewToken` takes the prefix (`issue` passes it by kind). |
| D17 | **Q5:** an admin that creates a space joins it as `admin`, and the space has no leader. **Q7:** the bootstrap admin is `admin`. **Q10:** reading other agents' inboxes stays out of scope. | PO rulings |

---

## 3. Behaviour overview

| Capability | Leader | Member | Admin, not joined | Admin, joined |
| :--- | :--- | :--- | :--- | :--- |
| See space list | all | own | all | all |
| Read participants, topics, boards, threads | own spaces | own space | every space | every space |
| Send direct / topic / `all` / critical | all four, in its spaces | direct and topic only | all four, every space | all four, every space |
| Create topic, board, thread; post | yes | yes | yes, **not subscribed** | yes, subscribed like anyone |
| Close topic, remove thread, delete board of others | yes | no | yes | yes |
| Invite; revoke members | yes | no | yes | yes |
| Revoke leaders from a space | no | no | yes, incl. the last leader | yes, incl. the last leader |
| Delete space | only as the sole leader | no | **any space** | **any space** |
| Create / join / leave space | yes / yes / not the last leader | — / — / yes | create (becomes joined) / join / `forbidden "not a participant…"` | — / conflict / yes, always. In every column, the last participant's departure deletes the space (D15) |
| Receive direct messages, `all`, subscriptions | yes | yes | **no** (`not_found "participant ops not in space demo"`) | yes |
| Inbox read/ack/stream | yes | yes | yes (only entries of joined spaces exist) | yes |
| Mint leader / admin, list / revoke admins | no | no | yes | yes |
| Shutdown (loopback) | no | no | yes | yes |
| Replace instance token (loopback) | no | no | bootstrap only | bootstrap only |

---

## 4. Seam comparisons

Ratings: 1 = poor, 5 = excellent.

### 4.1 Implicit leader everywhere

| Option | Description | Clean | Simple | Powerful | Verdict |
| :--- | :--- | :---: | :---: | :---: | :--- |
| **A. Resolver branch, synthetic `Role: leader`** | `ResolveTx` returns `Role: RoleLeader` for every admin, plus flags `Admin` and `Joined` | 3 | 5 | 4 | `Role` means two things for admins (authority vs. row). Every participation-role site (Leave last-leader count, revoke target, `summarize`, Join probe) has to remember the flags; a joined admin with role leader would block or allow "last leader" wrongly. |
| **B. Resolver branch, honest role + `Leads()`** *(recommended)* | `ResolveTx` admin branch: no member binding, no "not a participant" error; `Role` = the row's role (`admin`) or `""` when not joined; `Admin: true`. `Membership.Leads()` = `Admin \|\| Role == RoleLeader`; `Membership.Joined()` = `Role != ""` | **5** | 4 | **5** | The six authority sites change mechanically from `m.Role != RoleLeader` to `!m.Leads()`. Participation sites keep reading the row. Every future feature gets admin authority by resolving. |
| C. Real participant row for every admin in every space | Insert admin rows on space creation and admin mint | 1 | 2 | 3 | **Violates R2**: every admin would be messageable everywhere and appear in every list. |
| D. Per-service admin checks | `if p.Kind == auth.Admin` in each feature | 1 | 2 | 5 | Duplicates the gate the resolver exists for. |

**Recommendation: B.** It costs six one-word edits more than A. It also avoids a struct whose `Role` lies, which would otherwise need flag checks at more sites than it saves. Concretely:

```go
// membership.go
const RoleAdmin Role = "admin"
type Membership struct { SpaceID int64; SpaceName string; AgentID int64; AgentName string
    Role  Role // participant row role; "" when an admin has not joined
    Admin bool }
func (m Membership) Leads() bool  { return m.Admin || m.Role == RoleLeader }
func (m Membership) Joined() bool { return m.Role != "" }
```

Sites (the web §2.3 inventory of 37 places, re-mapped):

| Group | Sites | Change |
| :--- | :--- | :--- |
| Gate | `resolver.go:19-25,53-75` | Delete `requireAgent`. Admin branch: resolve the space, read the optional row, `Admin: true`. |
| Leader authority | `messaging/service.go:57`, `messaging/topics.go:79,124`, `blackboards/boards.go:55`, `blackboards/threads.go:108`, `spaces/service.go:95-100` | `m.Leads()` |
| Subscriptions | `messaging/topics.go:52,101-112`, `blackboards/threads.go:76`, `blackboards/posts.go:39-56` | Auto-subscribe only if `m.Joined()`; (un)subscribe requires `m.Joined()` → `forbidden "join space demo first; only participants receive messages"` |
| Space lifecycle | `spaces/service.go:103-138` (Create), `141-159` (Join), `162-180` (Delete), `183-200` (Leave) | Create/Join accept admins and insert `roleFor(p)` (`admin` or `leader`). Join tests `m.Joined()` instead of the forbidden probe. Delete skips `countOtherLeaders` when `m.Admin`. Leave refuses `!m.Joined()` with `forbidden "not a participant of space X"`; the last-leader rule keeps reading `m.Role`. |
| Revoke | `spaces/revoke.go:30-54,70-79` | Admin caller (`m.Admin`): leader targets allowed, including the last leader (`removeParticipant`; `RevokeSpaceTokens` deletes nothing for a NULL-space leader token). Leader caller: unchanged conflict for leader targets. Role `admin` target → `conflict "ops is an admin of space demo; admins cannot be revoked from a space"`. |
| Listing | `spaces/service.go:28-34,60-93`, `spaces/repository.go:145-146`, `spaces/whoami.go:24-43` | Drop `requireAgent` from List. `SpaceInfo.Admins []string`; members count only role `member`. Order `admin`, `leader`, `member`, then name. Whoami: admins get name and joined spaces. |
| Inbox | `inbox/service.go:74-104`, `inbox/handler.go:46-52` | Delete `requireAgent`; admins read across spaces (`spaceFilter` = 0) |
| Unchanged | departure hooks (`messaging/departure.go`, `blackboards/departure.go:17`: role `admin` ≠ `member` keeps boards), `Notify` sender (`membership.go:59`), `admission.go`, `mcpserver/verifier.go:27` (now `admin:<id>`) | — |

### 4.2 Admin identity

| Option | Clean | Simple | Powerful | Verdict |
| :--- | :---: | :---: | :---: | :--- |
| **A. Named admin agents; bootstrap `admin` ensured at startup** *(recommended)* | **5** | 4 | **5** | One identity per person or console: own inbox, own stream slot, own attribution, independent revocation. Uses the existing `ensureAgent`/`issue`/`agentOfKind` machinery unchanged. |
| B. One shared admin agent row, all admin tokens bound to it | 3 | 5 | 2 | One inbox and **one stream slot** for every admin session (409 collisions); no attribution; revoking one person needs per-token handles the API does not have. |
| C. Bootstrap without an agent row (no inbox), only minted admins are agents | 2 | 3 | 3 | Two admin species. The CLI/first-login identity has no inbox, which contradicts R1. |
| D. `@admin` reserved row (the superseded design) | 3 | 3 | 2 | Unaddressable by name, so R2 is impossible. Single identity. |

#### 4.2b Where the bootstrap credential lives

| Option | Clean | Simple | Powerful | Verdict |
| :--- | :---: | :---: | :---: | :--- |
| **In memory as today; `Authenticate` maps a match to the bootstrap agent** *(recommended)* | **5** | **5** | 4 | Env/file stays the single source of truth; `rotate-token` and `SetAdminToken` keep working; no DB sync. The cost is one extra field (`Principal.Bootstrap`) and a cached agent id. |
| Stored as a `tokens` row, re-synced at startup and on rotation | 3 | 3 | 4 | Two sources of truth. A file edited while the server is stopped needs a startup reconciliation that deletes "the old bootstrap row", which needs a marker column. |

#### 4.2c How the bootstrap row is created

| Option | Clean | Simple | Powerful | Verdict |
| :--- | :---: | :---: | :---: | :--- |
| **`auth.Service.Bootstrap(ctx)` at startup (ensure `admin`/kind admin, cache id)** *(recommended)* | 4 | 4 | 5 | Survives restarts, reuses `ensureAgent`. Test DBs stay free of it, so the fixtures that seed explicit ids from 1 keep working. |
| Seeded by `0001_schema.up.sql` (`INSERT … VALUES (1,'admin','admin',…)`) | 4 | 3 | 4 | Takes id 1 in **every** test DB, colliding with `dbtest.SeedAgent(t, conn, 1, …)` across the fixtures; Go must hard-code id 1. |

### 4.3 Old-database handling (decided by the PO)

**Ruling:** no handling in code. The reset is done as if this were the service's first iteration: `0001_schema.up.sql` replaces the old files and `internal/platform/db` stays unchanged. The only consequence for operators is one line in the release notes and in `server/README.md`/`DEPLOYMENT.md` (Upgrade): *"Pre-release databases are not upgraded: stop the server and delete `agent-message-bus.db`, `-wal` and `-shm` before starting this version."*

**Accepted risk (one line):** an old DB already records version 1, so the runner silently skips the new 0001, and the server fails later on the first admin write (`CHECK constraint failed: kind IN ('leader','member')`, observed in a probe against modernc.org/sqlite v1.61.0).

The options considered earlier (refusal with an `application_id` stamp, wipe, move aside) are dropped. The reset needs no table rebuild, unlike the superseded design (web §2.3).

---

## 5. Admin tool contracts

| Tool | Args | Result | Errors |
| :--- | :--- | :--- | :--- |
| `bus_mint_leader` | `{name}` | `{name, token}` | unchanged; now open to every admin |
| `bus_mint_admin` *(new)* | `{name}` | `{name, token}` | `forbidden "admin token required"` (checked before the name); `invalid` name; `conflict "agent X is a leader agent"` / `"… is a member agent"`; `conflict "admin is the bootstrap admin; its token is the instance admin token"` |
| `bus_list_admins` *(new)* | `{}` | `[{name, bootstrap, tokens, spaces}]` by name; `tokens` = minted tokens in the DB (0 for the bootstrap admin and for revoked admins); `spaces` = joined space names | `forbidden "admin token required"` |
| `bus_revoke_admin` *(new)* | `{name}` | `{deleted}`: true when tokens or participations were removed | `forbidden "admin token required"`; `forbidden "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"`; `conflict "agent X is a leader agent"` |

The kind-conflict message needs the right article for `admin`: `agent ops is an admin agent` (`auth/service.go:110` formats `a %s`).

Existing tool descriptions change their access line ("Participants only." → "Participants and admins."; "Leaders of the space only" → "Leaders of the space and admins", which includes the extension's `bus_spawn_worker`; `bus_list_spaces` "Leaders and admins see all spaces"; `bus_inbox_*` "Leaders, members and admins"; `bus_create_space`/`bus_join_space` accept admins; the `critical` property "Leaders and admins only"). `server/TOOLS.md` is regenerated. This slice adds 3 tools (29 → 32). The final registry after every proposal is 34 server tools; `docs/design/specs/REVIEW.md` §11 gives the count at each merge step.

---

## 6. Security

**6.1 The loopback rule.** It **still holds, unchanged, for `/v1/admin/*`**. Those are the instance lifecycle endpoints the local CLI uses. It is **not extended to `bus_mint_admin`/`bus_revoke_admin`/`bus_list_admins`**:
- The tool registry is transport-neutral (`internal/tools`, served by `/v1/tools` and `/mcp`). Tools never see a remote address, so a per-tool network rule would need new plumbing through both transports.
- Behind a reverse proxy every request is loopback (`spec/README.md:89`), so the rule protects nothing there. That is exactly where a website would run.
- An admin token over the network already confers leader power in every space, including deleting any space and minting leaders. Minting admins adds no new kind of power.

**6.2 What minting admins does add: persistence.** A stolen admin token can mint a durable admin that **survives `rotate-token`** (D3). Mitigations in the spec: `bus_list_admins` makes every admin visible; `bus_revoke_admin` removes it; the bootstrap admin can never be locked out (not revocable, its token lives on the host); every mint and revoke writes one audit log line (D14). Incident playbook for `server/DEPLOYMENT.md`: `rotate-token` → check the log for `admin minted` → `bus_list_admins` → `bus_revoke_admin` for unknown names. Minting admins from the web GUI is allowed (PO) and owned by the web BA.

**6.3 Instance token replacement is bootstrap-only (D12).** Otherwise a minted admin on the host could take over the CLI credential.

**6.4 Blast radius.** An admin is now strictly stronger than any leader. Each admin token is as sensitive as the instance token. Give each human or console its own minted admin so that revocation is surgical.

**6.5 Pre-existing gap, unchanged.** Deleting tokens does not end an inbox stream that is already open (the inbox spec has no such scenario, `server/spec/inbox.feature:232-247`). A revoked admin's stream lives until it disconnects. After revocation it carries nothing new, because the admin has no participations left.

---

## 7. Schema reset

**`server/migrations/0001_schema.up.sql`** = `0001_init.up.sql` + `0002_invites.up.sql`, with:
- `agents.kind CHECK (kind IN ('admin','leader','member'))`;
- `participants.role CHECK (role IN ('admin','leader','member'))`;
- the `tokens.space_id` comment becomes "NULL = leader or admin token";
- no PRAGMA, no seed row;
- a header comment: "Initial schema after the alpha reset. Not reversible: delete the database file to reset."
- the `invites.task` comment becomes "the member's task" (it is no longer "returned once on accept" after the invite-task slice).

No bootstrap row (§4.2c), no other change: the reset matches the current state plus the admin kind and role. Every other proposed schema change (`invites.invited_by`, the inbox notice columns, an invite id) is a numbered migration after `0001_schema` and is never folded into it (`docs/design/specs/REVIEW.md` §11).

**Deleted:** `0001_init.up.sql`, `0001_init.down.sql`, `0002_invites.up.sql`, `0002_invites.down.sql`. No `.down.sql` is written for the new file. The runner never read down files (`migrate.go:14`), and `.omp/AGENTS.md` ("reversible … or explicitly marked as not") is satisfied by the header comment. The `migrate_test.go:41` fixture that proves down files are ignored stays.

**`embed.go`:** unchanged (`//go:embed *.sql`).

**Runner:** unchanged (`internal/platform/db/migrate.go`, `dbtest`, `main.go:125`). A fresh DB gets version 1. `0002+` work as before.

**After the reset** the rule "every schema change is a new numbered migration; never edit an applied one" applies again from `0002_*`.

---

## 8. Change list per package

| Package / file | Change |
| :--- | :--- |
| `migrations/` | New `0001_schema.up.sql`; delete the four old files |
| `internal/platform/db`, `dbtest` | none (seed doc comments may name kind/role `admin`) |
| `internal/auth` | `Principal.Bootstrap`; `Service.Bootstrap(ctx)` (ensure `admin`, cache id); `Authenticate` maps the instance token to the bootstrap principal; `MintAdmin`, `ListAdmins` (read-only join on `participants`/`spaces` for `spaces`), `AdminAgent(ctx, q, name)` (id, found, bootstrap), `RevokeAgentTokens(ctx, q, agentID) (int64, error)`; article-aware kind-conflict message; `NewToken(prefix)` with `amb_admin_` for admin agents (D16). New file `admins.go` (+ repository queries). |
| `internal/spaces` | `RoleAdmin`; `Membership.Admin`, `Leads()`, `Joined()`; resolver admin branch, `requireAgent` deleted, `requireLeaderPrincipal` → accepts admins for create/join; Create/Join/Delete/Leave per §4.1; revoke: admins may revoke leaders (incl. the last), admin targets conflict; `removeParticipant` ends with "count participants; 0 → `deleteSpace`" (D15, new repository query `countParticipants`); `SpaceInfo.Admins`; participant order; Whoami for admins. New file `admins.go`: `RevokeAdmin(ctx, p, name)` in one `inbox.Deliver` tx: refuse bootstrap, `removeParticipant` for each joined space, `accounts.RevokeAgentTokens`. `memberAccounts` interface gains `AdminAgent` and `RevokeAgentTokens` (rename to `accounts`). |
| `internal/messaging` | `m.Leads()` at `service.go:57`, `topics.go:79,124`; `CreateTopic` subscribes only if `m.Joined()`; `changeSubscription` requires `m.Joined()` |
| `internal/blackboards` | `m.Leads()` at `boards.go:55`, `threads.go:108`; `CreateThread` subscribes only if joined; `posts.go` Subscribe/Unsubscribe require joined |
| `internal/inbox` | Delete `requireAgent` (service + stream handler) |
| `internal/tools` | `admin.go`: `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`; `deps.go` interfaces; access lines of existing descriptions; `bus_list_spaces` result shape `{name, leaders, admins, members, joined}`; regenerate `TOOLS.md` |
| `internal/admin` | `admin-token` handler requires `p.Bootstrap` (`forbidden "only the bootstrap admin may replace the instance admin token"`); shutdown unchanged |
| `internal/server` | `Build(ctx, streamRoot, conn, adminToken, shutdown) (http.Handler, error)` calls `authSvc.Bootstrap(ctx)`; `build_test.go` |
| `main.go` | Handle `Build`'s error; usage text: "admin-token print the bootstrap admin's token" |
| `internal/mcpserver` | none (`admin:<id>` follows from the agent id) |
| `server/spec/*.feature` | Merge the five proposal features; rewrite every "admin token cannot act in spaces / has no inbox" scenario (§1.6); `README.md` glossary (Admin, Admin token, Leader token), design decisions, known limitations, `no UI` line (web BA). **D15 impact on existing lines:** `membership.feature:137-140` keeps its outcome (a member is present), but its title should say "while other participants remain"; `spaces.feature`'s "sole remaining leader deletes the space" scenarios are unaffected. Additions: a new Rule in `membership.feature` (after the departure rule, `:154`) carrying `no-empty-spaces.feature`, including "the lone leader leaves and the space is deleted"; `spec/README.md:72` index text; a "No empty spaces" design decision in `README.md`. `auth.feature:57-61` ("amb_" format) stays for leaders and gains the `amb_admin_` admin case (D16). |
| Docs | `server/README.md` Tokens/endpoints/admin-token rows, `server/DEPLOYMENT.md` §3 token text, §7, Upgrade (the "delete pre-release databases" line, D13), incident playbook, troubleshooting rows; root `README.md` quick start; release notes |
| Audit (D14) | `slog.InfoContext` in the `bus_mint_admin` and `bus_revoke_admin` paths (auth `MintAdmin`, spaces `RevokeAdmin`), after commit only |
| `extension/` | Owned by the guidance and omp-parity BAs (§10) |

Suggested order: (1) schema reset (file swap only); (2) auth bootstrap principal + `Build` error; (3) membership seam B with authority sites, zero behaviour change for leaders and members; (4) admin in resolver, inbox, lifecycle, subscriptions, revoke; (5) admin tools + audit lines + endpoint guard; (6) specs, docs, `TOOLS.md`.

---

## 9. Risks

| # | Risk | Mitigation |
| :--- | :--- | :--- |
| K1 | `Membership.Role == ""` for an admin that has not joined. Any site that compares roles for display or logic and was missed misbehaves. | The §4.1 table lists every role site (grep `\.Role\b`); one test per site with an unjoined admin |
| K2 | Persistence through minted admins after token theft (§6.2) | List + revoke + log lines (Q4) |
| K3 | The bootstrap admin used by several omp sessions or the console → 409 on the second stream | Guidance: one minted admin per session/person; the extension already treats 409 as non-fatal |
| K4 | ~~Spaces with no participant~~ | **Gone:** D15 makes them impossible |
| K5 | A pre-reset DB silently skips the new 0001 and fails on the first admin write | **Accepted (PO).** Release-notes/README line telling operators to delete the DB |
| K6 | `Build` signature change | Small; `build_test.go` |
| K9 | An admin revoking the last leader leaves a space without leaders (when others remain) | Intended (D11): members keep working, admins lead it, leaders can join. If nobody remains, the space is deleted (D15). |
| K7 | Old extensions treat an admin token as "no tools". A new extension against an old server. | Old extension + new server: still works (admin stays tool-less). New extension + old server: admin tools fail as today. Accept for the alpha. |
| K8 | An unjoined admin's DMs cannot be answered | Specified on purpose (R2): the reply fails with `not_found`. The guidance tells members to report to a leader in that case. |

---

## 10. Open questions for the PO

**All closed.** Q1 delete any space (D10); Q2 revoke leaders, last one included (D11); Q3 any admin mints admins, and the web GUI may too (web BA); Q4 audit line per mint and revoke (D14); Q5 a creating admin joins as `admin`, the space is leaderless (D17); Q6 no empty spaces (D15); Q7 bootstrap name `admin` (D17); Q8 old DBs: manual deletion, risk accepted (D13); Q9 `amb_admin_` prefix (D16); Q10 other agents' inboxes out of scope (D17). Bootstrap-only instance-token replacement confirmed (D12).

---

## 11. Hand-off to the other BAs

### 11.1 Guidance BA (`docs/design/specs/guidance/`)

1. **Reopen Q11 (`analysis.md:347`).** An admin-token session is now a **full agent session**: bus tools active, **inbox stream opened**, the same inbox tools as a leader (per the omp policy: `bus_inbox_read`; `bus_inbox_ack` not offered in omp) and `bus_wait`. The precedence stays lowest (promote > env > accepted > stored agent token > admin token). Update `session-context.feature:74-106`, `analysis.md:12,144-150,291,305-306`, and drop "no inbox stream, no inbox tools, no `bus_wait`".
2. **Admin block text** (replace "`@admin` … not a participant … no inbox"): `You are admin \`<name>\` on the agent message bus at <url>. You act as a leader in every space without joining it.` then either `You joined: \`demo\`. Participants there can message you.` or `You joined no space: nobody can message you until you join one with bus_join_space.`, then `Spaces: …` and the guide.
3. **Guide "Admin" section (`guide-draft.md:29-30`, `agent-guide.feature:85-86`):** admins are named agents (bootstrap `admin`, more via `bus_mint_admin`). They lead every space without joining. Join a space to be reachable; leaving keeps authority. `bus_list_admins`/`bus_revoke_admin`. Leave the work to leaders.
4. **Member lines (`guide-draft.md:13,16-17`, `session-context.feature:41`, `agent-guide.feature:69,72`):** "a direct message from a leader or an admin is an order"; "CRITICAL entries come from a leader or an admin". Replace "`@admin` has no inbox: report its tasks to a leader" with "report to the sender; if that fails with `not_found` (an admin that has not joined), report to a leader of the space".
5. **`first-prompt.feature:21-22,40-42,81-84`:** the inviter is recorded by its admin **name** (e.g. `ops`), not `@admin`. A task from an admin that has not joined is reported to a leader.
6. **`space-overview.feature:113-118`:** `you` for an admin is `{"name":"ops","role":"admin"}` when joined and `{"name":"ops","role":null}` (or omit `role`) when not. Participants include the admin only if joined. The tool count adds the 3 admin tools.
7. **Subagent of an admin session:** "acts as admin `<name>`" instead of `@admin` (`session-context.feature:103-107`).
8. Drop every `@admin` mention and the "reserved agent row" notes (`analysis.md:345`, `guide-draft.md:45`).

### 11.2 Web BA (`docs/design/specs/web/`)

1. **Console identity = the admin agent behind the sign-in token**, shown by its name and kind (`admin admin` or `admin ops`), never `@admin`. In `console-access.feature:10-12,31,53-59`, replace the "reserved agent `@admin`" Background with "the bootstrap admin `admin` exists" and "signs in as admin `<name>`".
2. **Recommend a minted admin per human operator** (`bus_mint_admin`) for the console: it gives attribution, independent revocation, and keeps the bootstrap token on the host for the CLI. Accepted-risk text §4.2: rotation now only kills bootstrap sessions; `bus_revoke_admin` kills a minted admin's sessions (re-validation per request already covers both).
3. **`spaces-and-participants.feature:6-12,81-85`:** roles are `admin`, `leader`, `member`. An admin appears in a participant list **only if it joined**, listed first. The space list gains an `admins` column; members count only role `member`. Replace "the superuser never appears".
4. **`console-mutations.feature`:** author and sender are the operator's admin name (`:139,146-147,160-170`). The `@admin` invite scenario (`:90-94`) becomes "inviting an admin's name fails with conflict `agent ops is an admin agent`". The audit says "by `<admin name>`".
5. **Live updates:** the console still must not open the inbox stream. Admins now **have** a stream slot, and a console holding it would 409 the operator's omp session. An optional read-only "my inbox" page via `bus_inbox_read` is a new open question.
6. **New console area (R5, PO: the GUI may mint admins; the web BA owns it):** admin management (list, mint with the token shown once, revoke) backed by `bus_list_admins`/`bus_mint_admin`/`bus_revoke_admin`. Every mint and revoke is logged (D14). The console's revoke may now target leaders too, including the last leader (D11), so its confirmation step must name the role. Whether delete-space stays out of the console (their round-3 decision) is theirs to revisit, since admins may delete any space.
7. **`analysis.md`:** replace §2 with a pointer to `docs/design/specs/admin/analysis.md`. §4.1 item 1 becomes "instance token never in the DB; minted admin tokens hashed in `tokens`". Item 5 ("no inbox") is gone. §4.2 is unchanged in substance (the tool API has no loopback rule; the console is loopback-only by default). Close web Q1 (seam B), Q2 (admins create/join/leave), Q6 (schema reset), Q7 (whoami lists joined spaces) and Q8 (`admin:<id>`).
