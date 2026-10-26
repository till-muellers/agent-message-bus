# Schema — `0001_schema` reset and later numbered migrations

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix (e.g. `mail/analysis.md`) are relative to `docs/design/specs/`.

Sources: `server/migrations/0001_init.up.sql`, `server/migrations/0002_invites.up.sql` (current), `docs/design/specs/admin/analysis.md` §7 (lines 244-261), `docs/design/specs/admin/schema-reset.feature`, `docs/design/specs/REVIEW.md` §6 and §11, `docs/design/specs/guidance/analysis.md` (invited_by), `docs/design/specs/mail/analysis.md:157,251` (notice columns).

Rules (from slice 1 on, `.omp/AGENTS.md:96-97` as amended by X5):
- `0001_schema.up.sql` is the only reset file, written once in slice 1, marked irreversible by its header; no `.down.sql`. The runner never reads down files (`platform/db/migrate.go:14`).
- Everything after it is a new numbered migration in merge order with a `.down.sql`, never folded into 0001, never edited after merge.
- No seed rows: the bootstrap admin is created in Go by `auth.Service.Bootstrap` (slice 4a; `admin/analysis.md:199-200`), so `dbtest` fixtures that seed explicit ids from 1 keep working.
- Each migration runs in one transaction (`migrate.go:70-80`).

---

## 1. `server/migrations/0001_schema.up.sql` (slice 1)

Changes against today's two files, and nothing else (`admin/analysis.md:254`):
1. `agents.kind` CHECK gains `'admin'`.
2. `participants.role` CHECK gains `'admin'`.
3. Comment `tokens.space_id`: "NULL = leader or admin token".
4. Comment `invites.task`: "the member's task".
5. Header comment.
6. `invites` + `invites_space_name` folded in from `0002_invites.up.sql`.

```sql
-- Initial schema after the alpha reset. Not reversible: delete the database file to reset.
CREATE TABLE agents (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK (kind IN ('admin','leader','member')), created_at TEXT NOT NULL);
CREATE TABLE spaces (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
CREATE TABLE tokens (hash TEXT PRIMARY KEY,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  space_id INTEGER REFERENCES spaces(id) ON DELETE CASCADE, -- NULL = leader or admin token
  created_at TEXT NOT NULL);
CREATE TABLE participants (space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('admin','leader','member')), joined_at TEXT NOT NULL,
  PRIMARY KEY (space_id, agent_id));
CREATE TABLE topics (id INTEGER PRIMARY KEY, space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id),
  created_at TEXT NOT NULL, closed_at TEXT);
CREATE UNIQUE INDEX topics_open_name ON topics(space_id, name) WHERE closed_at IS NULL;
CREATE TABLE topic_subscriptions (topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE, PRIMARY KEY (topic_id, agent_id));
CREATE TABLE blackboards (id INTEGER PRIMARY KEY, space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL, owner_id INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL,
  UNIQUE (space_id, name));
CREATE TABLE threads (id INTEGER PRIMARY KEY, blackboard_id INTEGER NOT NULL REFERENCES blackboards(id) ON DELETE CASCADE,
  title TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
CREATE TABLE posts (id INTEGER PRIMARY KEY, thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  author_id INTEGER NOT NULL REFERENCES agents(id), body TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE thread_subscriptions (thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE, PRIMARY KEY (thread_id, agent_id));
CREATE TABLE inbox (id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  type TEXT NOT NULL, from_name TEXT NOT NULL, critical INTEGER NOT NULL DEFAULT 0,
  topic TEXT, blackboard TEXT, thread_id INTEGER, thread_title TEXT, body TEXT, created_at TEXT NOT NULL);
CREATE INDEX inbox_agent ON inbox(agent_id, id);
CREATE TABLE invites (
  hash TEXT PRIMARY KEY,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  task TEXT, -- the member's task
  expires_at INTEGER NOT NULL -- unix seconds, so SQL can compare
);
CREATE INDEX invites_space_name ON invites (space_id, name);
```

Files deleted in slice 1: `0001_init.up.sql`, `0001_init.down.sql`, `0002_invites.up.sql`, `0002_invites.down.sql`. `embed.go` (`//go:embed *.sql`) and the runner stay unchanged.

Properties the slice-1 tests check (`admin/schema-reset.feature:25-55`): fresh DB → `schema_migrations` = {1}; the 12 tables exist; `kind 'admin'` and `role 'admin'` insert, `'owner'` fails the CHECK; a second `Migrate` applies nothing; a fixture `0002_example.up.sql` applies once on top (fstest.MapFS, like `migrate_test.go`). The "bootstrap admin exists" step (`schema-reset.feature:32`) is checked in slice 4a, not 1.

Cascade map relied on by "no empty spaces" (slice 5, D15): deleting a `spaces` row removes `participants`, `tokens` with that `space_id` (member tokens), `topics` → `topic_subscriptions`, `blackboards` → `threads` → `posts`, `thread_subscriptions`, `inbox` rows of the space, `invites` of the space. `agents` rows are never deleted (authors stay valid: `spaces.created_by`, `topics.created_by`, `blackboards.owner_id`, `threads.created_by`, `posts.author_id` have no cascade).

Known accepted risk (PO, `schema-reset.feature:18-23`): a pre-reset DB already records version 1 → the new 0001 is skipped → first admin write fails `CHECK constraint failed`. Release note + README/DEPLOYMENT line: *"Pre-release databases are not upgraded: stop the server and delete `agent-message-bus.db`, `-wal` and `-shm` before starting this version."*

---

## 2. Later migrations (numbered in merge order)

**Numbering rule (merge time):** only `0001_schema` has a fixed number. Every later migration has a fixed **name** and gets its number when its slice merges: the next free number after the highest file on `main` (`NNNN_<name>`). The author renumbers on rebase if another migration merged first; no slice orders another for its number. Section headings below give names only; `NNNN` in the SQL file names is that merge-time number. Expected today (7 → 13a → 10): `0002_invites_invited_by`, `0003_inbox_notice`, `0004_invites_id`, but any merge order is valid because the three touch independent columns. The delivery class (slice D1, PO-5) needs **no** migration: it is derived from `inbox.type` and `inbox.critical` [AD-15] (§2.5).

### 2.1 `NNNN_invites_invited_by` (slice 7)

Owner: `invites`. Source: `guidance/analysis.md` change sketch (REVIEW F2).

```sql
-- NNNN_invites_invited_by.up.sql
-- The inviter's agent name (leader or admin); the task is delivered as a direct message from it.
ALTER TABLE invites ADD COLUMN invited_by TEXT NOT NULL DEFAULT '';
```

```sql
-- NNNN_invites_invited_by.down.sql
ALTER TABLE invites DROP COLUMN invited_by;
```

[AD-2] `NOT NULL DEFAULT ''` instead of nullable `TEXT`: `Create` always writes `p.AgentName`; only codes created before the migration (≤ 10 minutes, `invites/service.go:20`) read `''`. A name, not an agent id, so the task still names its inviter after the inviter left (`first-prompt.feature:81-87`). `DROP COLUMN` needs SQLite ≥ 3.35 (modernc.org/sqlite bundles a newer engine).

### 2.2 `NNNN_inbox_notice` (slice 13a) — PO-7 approved

Owner: `inbox`. Source: `mail/analysis.md:157,251`; `mail/mail-notice.feature`.

```sql
-- NNNN_inbox_notice.up.sql
-- edge: this entry turned its recipient's mailbox scope from empty to non-empty (or is a critical re-notify).
-- noticed: a channel told the agent (hint, stream) or bus_inbox_read/bus_wait returned the entry.
ALTER TABLE inbox ADD COLUMN edge INTEGER NOT NULL DEFAULT 0 CHECK (edge IN (0, 1));
ALTER TABLE inbox ADD COLUMN noticed INTEGER NOT NULL DEFAULT 0 CHECK (noticed IN (0, 1));
CREATE INDEX inbox_unnoticed ON inbox(agent_id, space_id) WHERE edge = 1 AND noticed = 0;
```

```sql
-- NNNN_inbox_notice.down.sql
DROP INDEX IF EXISTS inbox_unnoticed;
ALTER TABLE inbox DROP COLUMN noticed;
ALTER TABLE inbox DROP COLUMN edge;
```

Edge computation (in `inbox.insert`, inside the Deliver tx, per recipient scope, first entry of that scope in the tx only): `NOT EXISTS (SELECT 1 FROM inbox WHERE agent_id = ? AND (? = 0 OR space_id = ?) AND id < ?)` using the existing `inbox_agent(agent_id, id)` index; scope `space` = recipient's space for member-kind agents, 0 otherwise (kind read from `agents`). Critical entries are always edges (PO-7, critical re-notify). Existing rows default to "no edge", so a migrated mailbox stays quiet until it empties once — acceptable (alpha).

### 2.3 `NNNN_invites_id` (slice 10) — PO-3: invite cancel by id, web only

Owner: `invites`. Source: PO-3 ruling; `web/gui-invite-cancel.feature` (Spec-review). The id is the GUI's handle to list and cancel one pending code; the code itself stays hashed.

```sql
-- NNNN_invites_id.up.sql
-- Opaque handle of one pending invite code (16 lowercase hex chars), listed and cancelled by the web GUI.
ALTER TABLE invites ADD COLUMN id TEXT NOT NULL DEFAULT '';
UPDATE invites SET id = lower(hex(randomblob(8)));
CREATE UNIQUE INDEX invites_id ON invites(id);
```

```sql
-- NNNN_invites_id.down.sql
DROP INDEX IF EXISTS invites_id;
ALTER TABLE invites DROP COLUMN id;
```

- The backfill gives codes pending at migration time an id, so the unique index needs no partial clause and the GUI can cancel them too.
- `invites.Create` generates the id in Go (`crypto/rand`, 8 bytes, hex) in the same insert; a unique-index collision (2⁻⁶⁴ per pair) surfaces as an error and the invite is not created.
- Not a primary key: `hash` stays the PK because redemption looks codes up by hash (`invites/repository.go:33-47`).

### 2.4 Audit table — not planned (PO-1: log line only)

PO-1 ruled one server log line per GUI change, so no table. Kept only as the shape a later decision would take; owner would be `web`, which today owns no SQL, so it would need its own approval.

```sql
-- NNNN_operator_actions.up.sql
CREATE TABLE operator_actions (id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL, action TEXT NOT NULL, space TEXT, target TEXT, created_at TEXT NOT NULL);
CREATE INDEX operator_actions_created ON operator_actions(created_at);
```

### 2.5 Not migrations

| Item | Where it lives |
|---|---|
| Bootstrap admin `admin` | `auth.Service.Bootstrap` at startup (4a) |
| Delivery class (`interrupt`/`queue`/`deferred`) | derived by `inbox.ClassOf(type, critical)` on every scan/insert (D1) [AD-15]; store it only if a future entry needs a per-entry choice that the stored columns cannot express |
| Minted admin tokens | `tokens` rows, `space_id` NULL (6a) — no schema change |
| Presence / receipts | in memory in `inbox` (12) |
| Web sessions, CSRF tokens | in memory in `web` (W1a) |
| Protocol version | `internal/server/protocol.go` constant (7) |
| Guide | `internal/guide/guide.md`, `go:embed` (11) |
| `[via subagent]` attribution | body prefix written by the extension (`omp-parity/analysis.md:152`); a `posts.via` column is deferred (omp-parity Q12) |
