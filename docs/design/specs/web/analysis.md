# Web GUI for the bus — analysis (round 5)

Scope: TODO item "Later: *A simple web frontend* for spaces, participants, topics, blackboards and invites."

Rounds 2/3 designed an admin token as a superuser `@admin`. Round 4 replaced that
with the PO's **full admin role**: admins are named agents of kind `admin`, with their
own inbox, their own admin tools and leader rights in every space without joining.
They are reachable only in the spaces they joined. Round 5 makes the product a **GUI**
rather than a console and settles the mutations it offers.

**The authoritative admin specification is [`docs/design/specs/admin/`](../admin/)** — the
current state, the seam comparisons, the change list per package, the security
analysis and the PO's open questions. This document covers the **web** side only and
points there for everything else:

| Admin spec | Covers |
| :--- | :--- |
| [`admin-role.feature`](../admin/admin-role.feature) | An admin is a leader in every space without joining |
| [`admin-messaging.feature`](../admin/admin-messaging.feature) | Joining, being messaged, the admin's own inbox and stream slot, leaving |
| [`admin-tokens.feature`](../admin/admin-tokens.feature) | Bootstrap admin `admin`, `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`, rotation |
| [`analysis.md`](../admin/analysis.md) | Current state, seams, change list, security, PO questions |

What round 5 changes:

1. **The product is a GUI, not a console.** The name is renamed throughout, and the
   seam is reassessed in §3.3–3.6: an **embedded SPA in strict TypeScript, built with
   bun at release time, embedded via `go:embed`, with no runtime framework**. The
   JSON API the GUI needs is listed in §3.5.
2. **The GUI may mint admins.** The administration area does list, mint and revoke.
3. **Admins are a strict superset of leaders:** they revoke leaders too, and they may
   delete any space. The GUI's mutations follow (§3.8).
4. Unchanged from round 4: the identity is the admin agent behind the token, shown by
   its name; one minted admin per human operator is recommended; the signed-in
   admin's own inbox is shown read-only; `rotate-token` ends only the bootstrap
   admin's sessions and `bus_revoke_admin` ends a minted admin's.

The round-2/3 superuser material (the `@admin` reserved row, the participant-row
inventory, the sqlite rebuild probes) is **superseded** — see §2.

No production code was changed. The `.feature` files live in `docs/design/specs/web/`; when
the features are implemented they belong in `server/spec/` next to the other specs
(`server/spec/README.md:90` currently states "**No thread claiming and no UI.** Both
are out of scope" — that line has to change, and so do `spec/auth.feature`,
`spec/spaces.feature`, `spec/inbox.feature` and `spec/transports.feature`, which
encode today's admin restrictions).

---

## 1. Current-state map

### 1.1 Server shape and constraints

| Fact | Where |
| :--- | :--- |
| One static Go binary, `CGO_ENABLED=0`, SQLite via `modernc.org/sqlite` | `server/go.mod:1-9`, `server/README.md:3` |
| Only two direct dependencies (`go-sdk`, `sqlite`); **no new dependency without approval** | `server/go.mod:5-8`, `.omp/RULES.md` |
| `embed` is already used in the repo (tool ledger templates, migrations) — embedding static assets needs no new dependency | `server/internal/tools/doc.go:10-14`, `server/migrations/embed.go:9` |
| Single-writer SQLite, one connection, **foreign keys on** (`_pragma=foreign_keys(1)`) | `server/internal/platform/db/db.go:19` |
| Frontend asset build: the repo has Bun/TypeScript for the **omp extension**, but the Go server ships no assets today | `extension/`, `package.json` |
| Serve loop: `httpserver.Serve` shuts down gracefully, cancels streams | `server/internal/platform/httpserver/serve.go:17-40` |
| Default bind address `127.0.0.1:8080`; `AMB_ADDR` overrides it | `server/internal/platform/config/config.go:15-16,30-31` |
| No TLS in the binary; remote access is expected behind a reverse proxy | `server/DEPLOYMENT.md:§8` |

### 1.2 Transports (all mounted in one mux)

| Route | Auth | Where |
| :--- | :--- | :--- |
| `GET /healthz` | none | `server/internal/server/routes.go:23` |
| `POST /mcp` | bearer | `server/internal/server/routes.go:24` |
| `GET /v1/tools`, `POST /v1/tools/{name}` | bearer | `server/internal/server/routes.go:25-27`, `server/internal/rpc/handler.go:24-63` |
| `POST /v1/invites/accept` | invite code as bearer, no middleware | `server/internal/server/routes.go:28`, `server/internal/invites/handler.go:12-25` |
| `GET /v1/inbox/stream` (SSE) | bearer | `server/internal/server/routes.go:29`, `server/internal/inbox/handler.go:22-44` |
| `POST /v1/admin/shutdown`, `POST /v1/admin/admin-token` | admin token **and loopback caller** | `server/internal/server/routes.go:30`, `server/internal/admin/handler.go:22-25`, `server/internal/admin/guard.go:14-33` |

Composition root: `server/internal/server/build.go:21-37` wires services and calls `Routes`. `server/main.go:131-145` builds the `http.Server`.

### 1.3 Auth model

| Token | Powers today | Where |
| :--- | :--- | :--- |
| admin | `bus_mint_leader`, `bus_whoami`, the two `/v1/admin/*` endpoints (loopback only). **Cannot act in spaces at all** (`forbidden "admin token cannot act in spaces; …"`) and has no inbox. | `server/internal/auth/principal.go:11-16`, `server/internal/spaces/resolver.go:20-25`, `server/internal/inbox/service.go:99-104` |
| leader | creates/joins/deletes spaces, invites and revokes in spaces it leads, sends to `all`, sends critical. `bus_list_spaces` returns **all** spaces. | `server/internal/spaces/service.go:61-78`, `server/internal/spaces/resolver.go:53-70` |
| member | exactly one space; direct messages, topics, blackboards, inbox | `server/internal/auth/repository.go:49-58` |

> **This is the state *today*.** Under the admin role ([`docs/design/specs/admin/`](../admin/)) the
> admin row becomes: named agents of kind `admin`, each with its own inbox and admin
> tools, leader rights in every space without joining, and reachability only in the
> spaces they joined.

Bearer parsing and the middleware: `server/internal/auth/bearer.go`, `server/internal/auth/middleware.go:10-24`. The admin token is a hash compared in constant time (`server/internal/auth/service.go:31-44`); it is **not** a row in the `agents` table. Under the admin role it stays that way for the **instance** token (the bootstrap admin's credential is the env/file token), while **minted** admin tokens are hashed rows in `tokens`.

**The one authorization gate** — `spaces.Resolver.ResolveTx` (`server/internal/spaces/resolver.go:53-70`): every space-scoped operation (messaging, blackboards, invites, inbox) resolves the caller through it. `requireAgent` (`server/internal/spaces/resolver.go:20-25`) is what forbids the admin today; the admin role replaces it with an admin branch in the same place (admin analysis §4.1, seam B). This single choke point is why the admin role can be implemented in one place.

### 1.4 What data the bus exposes today (read side)

| Data | Operation | Gate | Where |
| :--- | :--- | :--- | :--- |
| Spaces + leader names + member count | `bus_list_spaces` | leaders: all spaces; members: own space; admin: forbidden | `server/internal/spaces/service.go:61-78` |
| Participants + role | `bus_list_participants` | participant of the space | `server/internal/spaces/service.go:222-228` |
| Open topics (+ creator, caller's subscription flag) | `bus_list_topics` | participant of the space | `server/internal/messaging/topics.go:25-31` |
| Blackboards (+ owner, thread count) | `bus_list_blackboards` | participant of the space | `server/internal/blackboards/boards.go:23-30` |
| Threads of a board (+ title, author, post count, last activity) | `bus_list_threads` | participant of the space | `server/internal/blackboards/threads.go:43-53` |
| Thread with all posts | `bus_read_thread` | participant of the space | `server/internal/blackboards/threads.go:82-100` |
| Identity of the caller | `bus_whoami` | any token | `server/internal/spaces/whoami.go:24` |
| Own inbox | `bus_inbox_read` / `bus_inbox_ack` | leader/member, own entries only | `server/internal/inbox/service.go:74-93` |

### 1.5 What does **not** exist yet (relevant gaps)

1. **No listing of pending invites.** The `invites` table stores only the **hash** of a code (`server/migrations/0002_invites.up.sql`, `server/internal/invites/repository.go:22-31`); the plaintext code is returned exactly once by `bus_invite_member` (`server/internal/tools/invites.go:24-32`). Codes expire after 10 minutes (`server/internal/invites/service.go:19-20`, `TTL`). A read path that enumerates pending invites is needed — the invites feature owns that table.
2. **No agent row for the admin.** The `agents` table has `kind TEXT NOT NULL CHECK (kind IN ('leader','member'))` (`server/migrations/0001_init.up.sql:1-2`), and the admin token is not stored at all. Everything that writes an author uses `agents.id`: topics (`internal/messaging/repository.go:30`), blackboards (`internal/blackboards/repository.go:39`), threads (`internal/blackboards/repository.go:111`), posts (`internal/blackboards/repository.go:120`), topic subscriptions (`internal/messaging/repository.go:39`), thread subscriptions (`internal/blackboards/repository.go:179`), participants (`internal/spaces/repository.go:79`). With foreign keys on, an admin that writes needs a real agent row to write through — under the admin role that is the named admin agent.
3. **No cross-space read for the admin token.** The admin token is refused by the space resolver, so an operator holding only the admin token sees *no* spaces today. Under the admin role an admin sees every space without joining.
4. **No live channel for structure.** The only SSE endpoint is the per-agent inbox stream, and the server allows **one stream per agent** (`server/internal/inbox/handler.go:32-33`, `server/internal/inbox/slots.go`). The extension already holds that slot for the session's leader. A web GUI must not compete for it — and under the admin role the same holds for an admin's own slot.
5. **Topic messages have no history.** Messages live only in the recipients' inboxes and are deleted on ack; only blackboard posts are persistent history (`server/spec/README.md` glossary, "Thread / post").
6. **No admin management tools yet.** `bus_mint_admin`, `bus_list_admins` and `bus_revoke_admin` are specified in [`docs/design/specs/admin/admin-tokens.feature`](../admin/admin-tokens.feature) but do not exist in the code. The web GUI's administration area needs all three.
7. **No listing of the admins of an instance.** There is no read path for the admin list; `bus_list_admins` (admin spec) is the one that will provide it.

---

## 2. The admin role — superseded here, specified in `docs/design/specs/admin/`

> **Superseded.** The "admin token is a superuser `@admin`" design of rounds 2 and 3 is
> gone. The PO replaced it with a **full admin role**: admins are *named agents* of
> kind `admin`, each with its own inbox, admin tools and leader rights in every space
> without joining. The authoritative specification is in [`docs/design/specs/admin/`](../admin/):
>
> | File | Covers |
> | :--- | :--- |
> | [`admin-role.feature`](../admin/admin-role.feature) | An admin is a leader in every space without joining; reads, sends, posts, closes, removes, invites, revokes, creates and deletes spaces |
> | [`admin-messaging.feature`](../admin/admin-messaging.feature) | Joining (participant role `admin`), being messaged, `all` and subscriptions, the admin's own inbox and stream slot, leaving |
> | [`admin-tokens.feature`](../admin/admin-tokens.feature) | Bootstrap admin `admin`, `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`, rotation, the admin endpoints |
> | [`schema-reset.feature`](../admin/schema-reset.feature) | One fresh schema; no detection of old databases (they are deleted by hand) |
> | [`analysis.md`](../admin/analysis.md) | Current state, seam comparisons, the change list per package, the security analysis and the PO's open questions |
>
> The web side of the ruling is in this document: the web GUI identity (§3), the
> administration area (§3.9) and the security consequences (§4).

### 2.1 What the web GUI needs from the admin role

The parts of the admin ruling the web GUI depends on, in the web GUI's own terms:

| Admin capability | What the web GUI does with it |
| :--- | :--- |
| Named admin agent behind the token | The web GUI shows the operator as **admin `<name>`** — "admin admin" for the bootstrap admin, "admin ops" for a minted one. Never a fixed placeholder. |
| Leader rights in every space without joining | The web GUI's space list and every space page work for the operator, whichever space it is in. |
| Own inbox | The web GUI can show the operator's inbox — read-only (§3.10). |
| `bus_mint_admin` / `bus_list_admins` / `bus_revoke_admin` | The administration area of the web GUI (§3.9). |
| `bus_revoke_admin` ends that admin's sessions | The web GUI session of a revoked admin dies on its next request. |
| `rotate-token` ends only the bootstrap admin's sessions | The web GUI sessions of minted admins survive an instance-token rotation. |

**The participant-row inventory and the sqlite rebuild probes** of rounds 2 and 3
(§2.2 and §2.3 of the previous revision of this file) are **superseded**: the admin
role needs no `@admin` reserved row, no `agents.kind` rebuild and no `foreign_keys`
handling, because the schema reset in [`docs/design/specs/admin/`](../admin/analysis.md) §7
adds `admin` to `agents.kind` and `participants.role` in one fresh
`0001_schema.up.sql`. The current-state facts of §1 below are unaffected.
## 3. The web frontend

### 3.1 Who the operator is

The web GUI operator signs in with an **admin token**. Admins are named agents of kind
`admin` (see [`docs/design/specs/admin/`](../admin/)), so the web GUI shows the operator as
**admin `<name>`** — "admin admin" for the bootstrap admin, "admin ops" for a minted
one. Never a fixed placeholder: the name is what makes the operator's actions
attributable.

Because an admin leads every space without joining it, the web GUI shows **every
space**. The round-1 trade ("show only the spaces my token participates in") is
dropped, and the space list, the participant list, the topics, the blackboards and
the open invites of every space are reachable from one credential.

Round 1's other trades survive unchanged:

- **"Live updates" → "polling a read-only snapshot"**: the bus has exactly one live channel and it is single-consumer per agent; polling cannot steal a stream slot. Admins have a stream slot too, so the web GUI must leave it free as well.
- **"Show pending invite codes" → "show pending invite names and expiry"**: the plaintext code does not exist in the database (only its hash) and expires after ten minutes. The same holds for a minted admin token: it is shown once and never again.
- **Mutations are Phase 2** — see 3.8.

### 3.2 Recommendation: one minted admin per human operator

**Every human operator (and every web GUI) should get its own minted admin**, not
share the bootstrap admin's token:

| | Shared bootstrap token | One minted admin per operator |
| :--- | :--- | :--- |
| Attribution | every action is "admin admin" | the action is "admin ops" — the name of the person |
| Revocation | only `rotate-token`, which breaks the CLI's own credential | `bus_revoke_admin` for that one operator; the others keep working |
| Blast radius of a leak | the whole instance, including minting and shutdown | that operator's power over every space |
| The CLI | the instance token stays on the host, untouched | same |

The bootstrap admin's instance token stays on the host for `stop` and `rotate-token`
([`docs/design/specs/admin/analysis.md`](../admin/analysis.md) D3), which is the reason the
separation matters: **rotating the instance token ends only the bootstrap admin's
sessions** (the web GUI sessions of minted admins survive), and **`bus_revoke_admin`
ends a minted admin's sessions**. Both fall out of the web GUI re-validating the
token on every request.

### 3.3 Seam comparison: what kind of GUI?

The PO asked "why console? I'd like a proper GUI." A *console* (server-rendered pages,
one action per page load) and a *GUI* (a client-side application with live tables,
client-side routing, instant feedback) are different products, and they have
different costs. Four candidates:

| Option | Clean | Simple | Powerful | One static Go binary? | New dependency? |
| :--- | :---: | :---: | :---: | :--- | :--- |
| **(a) Server-rendered `html/template` + small vanilla-JS enhancement** | 5 | 5 | 2 | yes, trivially | no |
| **(b) Embedded SPA, strict TypeScript, built with bun, embedded via `go:embed`, no runtime framework** | 4 | 3 | 4 | yes | **no** — bun and TypeScript are already devDependencies |
| **(c) Embedded SPA with a UI framework (Preact/Svelte/React)** | 3 | 2 | 5 | yes | **yes** — needs PO approval |
| **(d) Desktop app (Tauri/Wails/Electron)** | 2 | 1 | 4 | **no** — a second binary | **yes** |

**Option (a) — server-rendered.** Every interaction is a page load or a POST that
returns HTML. `html/template` is stdlib, so there is no build step, no asset pipeline,
no CI change and no release change. The cost is the product: no client-side routing,
no instant table updates, no optimistic feedback, and every "live" view needs a
polling loop anyway. For an administration surface with an inbox, an admin list and
per-space tables, that is a genuinely worse experience than the PO is asking for. It
is the right answer only if the "GUI" is really a "read-only status page".

**Option (b) — embedded SPA in strict TypeScript, no runtime framework.** The
repository already speaks this language: `extension/` is strict TypeScript
(`tsconfig.json`: `strict`, `noEmit`, `verbatimModuleSyntax`), tested with `bun test`,
typechecked with `tsc --noEmit`, and CI already has a `typescript` job
(`.github/workflows/ci.yml`). The GUI would live next to it, be built by `bun build`
into a `dist/` directory, and be embedded with `//go:embed`. Consequences:

- **No new dependency.** bun and TypeScript are already `devDependencies`
  (`package.json`). "No runtime framework" means the shipped artefact is plain
  JavaScript; the *build* tool is the one the repo already uses.
- **One static Go binary, still `CGO_ENABLED=0`.** The GUI is bytes inside the
  binary; nothing changes for the Go build, the Docker image
  (`server/Dockerfile` copies the built binary) or the release artefacts.
- **But the build order changes.** `go:embed` needs the built assets to exist *before*
  `go build`. `scripts/build.py` (which runs `go build` six times for the release
  matrix) and the Docker build would have to run the GUI build first. Two ways out:
  1. *Commit the built assets* (`server/internal/web/dist/` checked in) — the Go build
     stays a plain `go build`, but the repo carries generated files and they drift.
  2. *Build the GUI in CI and in `scripts/build.py` before `go build`* — the repo stays
     clean, but `go build` on a fresh checkout now needs `bun install && bun run
     build:gui` first, and `go test ./...` on a fresh checkout must still work
     (it would, if the embed target has a checked-in placeholder or the tests do not
     need the assets).
- **The CI matrix grows by one step, not by one job.** The existing `typescript` job
  already installs bun and runs `tsc --noEmit` and `bun test`; the GUI needs the same
  two commands plus a build. The `release` workflow needs a bun setup step and a GUI
  build before `python scripts/build.py`.
- **Live updates need a JSON API**, because an SPA cannot call `html/template`
  handlers (§3.5). The existing `POST /v1/tools/{name}` already is such an API for
  everything the tools cover — the GUI can call it with the session's token if the
  server proxies it. But it cannot be called directly from the browser with a
  cookie session (the tool API is bearer-only), so the web transport needs a thin
  JSON façade anyway.

**Option (c) — SPA with a UI framework.** A framework buys component structure,
routing and reactive state, which is exactly what "a proper GUI" means. It costs a
**new npm dependency** (Preact ~4 kB, Svelte a compiler, React ~45 kB) and therefore
**PO approval** per `.omp/RULES.md` ("No new dependency without asking first"). The
build cost is the same as (b); only the runtime and the dependency list differ.
Given the GUI's size (five or six views), Preact would be proportionate; React would
be excessive for a local admin tool.

**Option (d) — desktop app.** A second binary (Tauri needs Rust, Wails needs a
webview and a CGO-capable toolchain, Electron needs Node and a ~100 MB runtime).
It breaks the repo's "one static Go binary" promise, the Docker image, the six-target
release matrix and the `scripts/build.py` flow, and it buys nothing that an embedded
GUI in the browser does not.

### 3.4 Recommendation

**Option (b): an embedded SPA in strict TypeScript, built with bun at release time,
embedded via `go:embed`, with no runtime framework.**

Why:

1. **It is what the PO asked for.** A client-side application with routing, live
   tables and instant feedback is a GUI; server-rendered pages are a console. The
   PO's question was aimed exactly at this difference.
2. **It costs no new dependency.** bun, TypeScript, `tsc --noEmit` and `bun test` are
   already the repo's TypeScript toolchain (`package.json`, `tsconfig.json`,
   `.github/workflows/ci.yml`). Option (c) is the same work *plus* an approval
   gate; option (b) is the same work without one.
3. **It keeps the "one static Go binary, `CGO_ENABLED=0`" promise.** The GUI is
   embedded bytes; the Go build, the Docker image and the six release targets are
   untouched apart from running the GUI build first.
4. **It is honest about the pipeline cost.** The GUI build must run before `go build`.
   That is a real change to `scripts/build.py`, the Docker build and the release
   workflow, and it must be visible in the plan rather than discovered later.

What the PO must approve:

| # | Approval | Why |
| :--- | :--- | :--- |
| 1 | **A build step in the release pipeline**: `bun install --frozen-lockfile && bun run build:gui` before `go build`, in `scripts/build.py`, in the Docker build and in `.github/workflows/release.yml` | The GUI must be embedded, so its bytes must exist before the Go compile. This is a pipeline change, not a dependency. |
| 2 | **The CI matrix gets a GUI build+test step** (in the existing `typescript` job) | The GUI is source code that must be typechecked and tested like the extension. |
| 3 | **The GUI's own source directory** | See §3.6: `gui/` at the repo root, next to `extension/`, so the Go module stays free of `node_modules` and the two TypeScript projects stay separate. |
| 4 | *(only if the PO prefers a framework)* **A new npm dependency** — Preact, not React | Option (c). Not needed for the recommendation. |
| 5 | *(only if the PO insists on a desktop app)* **A second binary and a second toolchain** | Option (d), which breaks the single-binary promise. Not recommended. |

No Go dependency is needed in any case: `embed`, `html/template` and `net/http` are
stdlib.

### 3.5 What the GUI needs from the server: the JSON API

The GUI is a client, so it needs JSON. The good news: **the tool registry already is a
JSON API**, and the GUI's reads and writes map onto it almost one-to-one.

**Reuse: `POST /v1/tools/{name}`** — the same transport the agents and the MCP
clients use, with the same arguments, the same result shapes and the same error
envelope (`{"result": …}` / `{"error":{"code","message"}}`,
`server/internal/rpc/handler.go:40-63`). The GUI's web transport **proxies** these
calls with the session's token, so the browser never holds the token and the rules
are enforced by the same services as everywhere else.

| GUI view / action | Tool call | Notes |
| :--- | :--- | :--- |
| Who am I | `bus_whoami` | kind, name, joined spaces |
| Space list | `bus_list_spaces` | per space: leaders, admins, members, joined |
| Participants of a space | `bus_list_participants` | |
| Topics of a space | `bus_list_topics` | |
| Blackboards of a space | `bus_list_blackboards` | |
| Threads of a blackboard | `bus_list_threads` | |
| A thread with its posts | `bus_read_thread` | |
| The admins of the instance | `bus_list_admins` | needs the admin spec's tool |
| The signed-in admin's inbox | inbox service read-only listing (not `bus_inbox_read`) | read-only in the GUI; never `bus_inbox_ack`; does not mark mail edges as noticed (`docs/design/specs/mail/mail-hint.feature`) |
| Create an invite | `bus_invite_member` | the code is shown once |
| Revoke a member or a leader | `bus_revoke_member` | admins are a strict superset of leaders |
| Post to a thread | `bus_add_post` | |
| Send to a topic | `bus_send_topic` | including critical and `all` |
| Delete a space | `bus_delete_space` | admins may delete any space |
| Mint an admin | `bus_mint_admin` | the token is shown once |
| Revoke an admin | `bus_revoke_admin` | minted admins only |

**New endpoints the GUI needs** (these do not exist and cannot be expressed as tool
calls):

| Method and path | Purpose | Why not a tool |
| :--- | :--- | :--- |
| `POST /web/session` | exchange an admin token for the session cookie; returns the admin's identity and the CSRF token | A session is transport state, not a bus operation |
| `DELETE /web/session` | sign out and drop the cookie | same |
| `GET /web/api/state?space=<name>` | **one aggregated read per view**: space, participants, topics, blackboards, threads, open invites, the admin's inbox count | An SPA would otherwise fire six requests per view and re-render six times. This is the GUI's main performance lever. It composes `internal/overview` (the read behind `bus_space_overview`, `docs/design/specs/guidance/space-overview.feature`) with `invites.ListPending`. It reads the same services; it adds no rules. |
| `GET /web/api/events` | **SSE of change hints** (not inbox entries): "space X changed", "admins changed", "inbox changed" | The existing `GET /v1/inbox/stream` is per-agent and single-consumer; a browser watching it would take the slot of the admin's own session and 409 it. The GUI needs a *broadcast* of "something changed", scoped to the session, that carries no message bodies. |

Only two of these are genuinely new server capabilities, and one of them is optional:

- **`GET /web/api/state`** reuses the guidance BA's `internal/overview` composition and
  needs one new read path: the pending invites of a space. There is no tool that lists
  them, because the invites feature owns that table and stores only hashes
  (`server/internal/invites/repository.go:22-31`); it needs a `ListPending` read
  method returning an opaque invite id, name, expiry and inviter. **PO-3 (round 6):**
  cancelling is not a tool; the GUI cancels by id through
  `DELETE /web/api/spaces/{space}/invites/{id}` (`gui-invite-cancel.feature`). The id
  column comes from a numbered migration after `0001_schema`.
- **The tool proxy drops the `mail` field.** If the "You have mail" hint lands
  (`docs/design/specs/mail/mail-hint.feature`), `/v1/tools` results may carry an additive
  `mail` field; the GUI proxy ignores it, and the GUI never reads the inbox through a
  tool, so it never clears the operator's hint.
- **`GET /web/api/events`** is the later upgrade and needs a new mechanism: the
  bus's broker is in-process pub/sub keyed by `inbox/<agentID>`
  (`server/internal/platform/broker/memory.go:18-36`), so a GUI broadcast needs a
  second topic family (e.g. `space/<spaceID>`) published by the feature services.
  **Recommendation: start with polling** (5 s interval on `GET /web/api/state`) and
  add the SSE broadcast only if the polling cost shows up; that keeps milestone 1
  free of a new event channel.

The GUI's own static assets are served by the same transport: `GET /web/*` for the
embedded SPA and `GET /web/assets/*` for its hashed bundles. `go:embed` cannot reach
outside the Go package, so the built assets live in `server/internal/web/dist/`
(§3.6).

### 3.6 Repo layout for the GUI

```
gui/                        the GUI source (strict TypeScript, like extension/)
  index.html                the shell
  src/
    api.ts                  the client: session cookie + CSRF header + fetch
    state.ts                client state, no framework
    views/                  one module per view (spaces, space, admins, inbox, login)
  package.json?             no — the repo root package.json is the plugin's
  tsconfig.json             extends the root settings, includes only gui/
```

Two decisions here:

- **The GUI lives at the repo root, next to `extension/`**, not inside `server/`.
  The Go module must stay free of `node_modules` (which `.gitignore` covers anyway,
  but the *directory* should not be inside the module), and the two TypeScript
  projects should not share a `package.json`: the root one is the omp plugin
  (`@till-muellers/omp-agent-message-bus`, installed from GitHub as a git dependency;
  no npm), and the GUI is not part of that package. The GUI needs
  its **own** `package.json` and `tsconfig.json`, and its own `bun.lock`.
- **The build output is embedded from inside the Go module.** `go:embed` cannot reach
  outside the package directory, so the built assets must land in
  `server/internal/web/dist/`. The build is `bun build gui/src → server/internal/web/dist`,
  and `server/internal/web/embed.go` embeds `dist/`. The `dist/` path is already
  gitignored at the repo root (`.gitignore:2`), so the same pattern inside `server/`
  keeps generated files out of the repo — which means `go build` needs the assets to
  exist. **Recommendation:** commit a tiny placeholder `dist/index.html` so that
  `go build` and `go test` always work on a fresh checkout, and let the release
  pipeline overwrite it with the real bundle. The alternative (no placeholder) breaks
  `go build` for every contributor without bun.

### 3.7 Auth and live updates in the GUI

| | Server-rendered (a) | Embedded SPA (b, recommended) |
| :--- | :--- | :--- |
| Credential in the browser | none (the cookie is HttpOnly) | none (the cookie is HttpOnly) — **not** a bearer token in `localStorage` |
| Session | cookie | cookie, same |
| CSRF | needed on every POST | needed on every POST (a custom header the SPA sets) |
| Live updates | server pushes HTML or the page polls | the SPA polls `GET /web/api/state`, or an SSE of change hints |
| The admin's inbox stream | untouched | untouched — the GUI never opens `GET /v1/inbox/stream`, because that would take the admin's single stream slot and 409 the operator's own omp session |

The bearer-token-in-memory alternative (the SPA keeps the admin token in a JS variable
and calls `POST /v1/tools/{name}` directly) was rejected in round 3 and stays
rejected: it puts the instance's highest-value credential inside the page, where any
XSS can read it, and it would need CORS for a dev-server setup. The cookie session
keeps the token server-side.

### 3.8 Re-assessing the Phase 2 mutations

Round 1 deferred invite creation and member revoke because the operator's credential was weak and because a cookie-authenticated mutation needs a CSRF model. The admin role and the round-5 ruling change both halves:

| | Round 1 | Round 5 (admin operator, admins ⊃ leaders) |
| :--- | :--- | :--- |
| Who can act | only in spaces the token participates in | **every space, board and topic** (an admin leads every space without joining) |
| Invite creation | needs a leader of that space | an admin can invite into every space |
| Revoke | needs a leader of that space | an admin revokes **members and leaders** of every space (round-5 ruling: admins are a strict superset of leaders) |
| Delete a space | only as the sole leader | an admin may delete **any** space (round-5 ruling) |
| Post / send | not offered | an admin may post and send, including critical messages and to `all` |
| Attribution | — | every action carries the operator's admin **name** |
| Risk of a mistake | a leader breaks one space | the operator can break **any** space, from the browser |

**Recommendation: Phase 2 offers five space mutations — create invite, revoke member or leader, post to a thread, send to a topic, delete a space — plus the administration area of 3.9.**

- **Create invite** is now natural for an admin: the operator is the one who knows which worker should join. The code is shown once and never again (it is stored hashed), which fits a GUI panel that displays it once.
- **Post to a thread** and **send to a topic** are the operator's steering instruments, and an admin may do both in every space. They are non-destructive and their effect is visible in the same view.
- **Revoke member or leader** is destructive, and an admin can do it in *any* space by accident. Revoking a member deletes its token for that space, its inbox entries there and its blackboards. Revoking a leader removes only its participation and its inbox entries in that space: its leader tokens stay valid and its boards stay (`docs/design/specs/admin/admin-role.feature`). If nobody remains, the space is deleted (`docs/design/specs/admin/no-empty-spaces.feature`). It therefore needs an explicit confirm step that names the space, the target and its role.
- **Delete a space** is the most destructive of all: it removes the space with everything in it. The round-5 ruling gives the admin the power; the GUI should still make it deliberate — a confirm step that names the space, and an audit line. It is in the GUI because the ruling puts the admin in charge of the instance, and an operator who cannot delete a space from the administration website has to drop to a terminal.
- **Deleting a board or a thread stays out of the web GUI.** Both are irreversible and neither needs a browser; they stay in the agent tools and the CLI.
- **CSRF becomes mandatory** in Phase 2, because the web GUI authenticates with a cookie. A per-session CSRF token bound to the session, checked on every mutating request, is the minimum. `SameSite=Strict` alone is not enough for a state-changing POST from the same site.
- **Audit (PO-1, decided).** Every GUI mutation writes one server log line with the admin name, the space, the target and the action; the server also logs every admin mint and revoke. No audit table, no "operator actions" list.

`gui-mutations.feature` specifies the Phase 2 behaviour of the space mutations.

### 3.9 The administration area

The PO wants a "full administration website". The web GUI therefore gets a third area
next to the spaces and the inbox: **the admins of the instance**, backed by the three
admin tools.

- **List** (`bus_list_admins`): every admin with its bootstrap flag, the number of
  tokens it holds and the spaces it joined. This is also the incident view: after
  `rotate-token`, the operator looks here for unknown admins and revokes them.
- **Mint** (`bus_mint_admin`): the web GUI **may mint admins** (PO ruling, round 5) —
  it is part of the administration website. The token is shown **once** — it is
  stored hashed and can never be shown again, exactly like an invite code. The GUI
  says so, and says where to put it (the GUI login form, or `credentials.json` for
  an omp session).
- **Revoke** (`bus_revoke_admin`): only a *minted* admin can be revoked; the
  bootstrap admin cannot (`forbidden "admin is the bootstrap admin; rotate its token
  with agent-message-bus rotate-token"`). A revoke ends that admin's web GUI sessions
  and its tokens.

The area is **admin-only**: a leader or member session gets 403. That is a different
gate from the space pages, which the admin role opens up.

`gui-admins.feature` specifies it.

### 3.10 The signed-in admin's own inbox — decision

**Decision: milestone 1 shows the signed-in admin's own inbox, read-only.**

The reasoning:

- Admins have their own inbox ([`docs/design/specs/admin/admin-messaging.feature`](../admin/admin-messaging.feature)), and the web GUI operator is an admin. An administration view that cannot show what was sent to the operator is missing the most obvious question ("did anyone ask me something?").
- The inbox holds the entries of the spaces the admin **joined**. An operator who wants to be reachable joins the space; the web GUI can show that, and the web GUI can show the operator's joined spaces.
- **Read-only, strictly.** The web GUI never acknowledges (`bus_inbox_ack` deletes entries) and never opens the inbox stream. That matters twice over: the operator's own omp session may be streaming the same admin's inbox, and a web GUI-held stream slot would 409 it. The web GUI shows the entries; clearing them is the agent session's job.
- **Only the signed-in admin's inbox.** The web GUI never shows another agent's inbox. (Admin Q10 keeps cross-agent inbox reads out of scope entirely.)

`gui-inbox.feature` specifies it.

### 3.11 Reading rules the web GUI must not drift from

- space list for a leader shows all spaces, for a member only its own; for an admin all spaces with their counts, and admins are counted separately from leaders and members (`docs/design/specs/admin/admin-messaging.feature`, "Listing spaces");
- participants are admins first, then leaders, then members, each by name;
- topics are `all` first, then by name (`server/internal/messaging/repository.go:81`);
- threads are most recently active first (`server/internal/blackboards/repository.go:136-140`);
- errors are the same codes and messages: `forbidden`, `not_found`, `invalid`, `conflict`, `unauthenticated` (`server/internal/platform/httpx/json.go:11-17`, `server/internal/platform/apperr/apperr.go:13-19`).

---

## 4. Security consequences of the admin role

### 4.1 An admin token is the highest-value secret in the system

Before the ruling the admin token could only mint leader tokens and shut down the
instance. Now it is a full agent identity with leader rights in every space:

- **read every space**: every participant list, every topic, every blackboard, every thread, every post — i.e. all the durable content of the instance (`server/internal/blackboards/repository.go:158-176`);
- **write as itself anywhere**: send direct messages, post to threads, create boards and topics — attributed to the admin's **name**, so recipients see who is speaking;
- **destroy**: revoke members of every space, delete any blackboard, thread or **space**;
- **mint admins**: create durable admin identities that survive `rotate-token`;
- **mint leaders**: create a new leader identity at will.

Compare with a leader token today: a leader sees only the spaces it participates in
and can act only there. An admin token is strictly stronger than every leader token.

Mitigations that must ship with the web GUI:

1. **The instance token is never in the database.** It stays in memory as a hash (`server/internal/auth/service.go:16-28,31-35`) and is not a `tokens` row. Minted admin tokens *are* hashed rows, so they survive a rotation — that is why the admin list and `bus_revoke_admin` matter ([`docs/design/specs/admin/analysis.md`](../admin/analysis.md) §6.2).
2. **Minimum length and rotation** stay as they are (≥32 chars, `rotate-token`, live swap `server/internal/admin/handler.go:34-52`, `server/internal/auth/admin_token.go:11-21`). Rotation ends **only the bootstrap admin's sessions** — the web GUI re-validates the token on every request, so that is automatic. A minted admin's sessions survive; they die with `bus_revoke_admin`.
3. **Audit admin writes (PO-1).** One server log line per GUI mutation (admin name, space, target, action) and per admin mint and revoke. No table, no actions list. Today the server logs errors only (`server/internal/platform/httpx/json.go:40-46`).
4. **One minted admin per operator** (3.2) turns a leaked web GUI credential from "the whole instance" into "one person's admin powers, revocable in one click".
5. **The admin's inbox is its own**, and the web GUI only ever shows the signed-in admin's inbox, read-only. It never acknowledges and never streams, so it cannot delete an entry the operator's omp session still has to handle, and it cannot take the admin's stream slot and 409 that session.
6. **The web GUI never shows a token twice** — not an admin token, not an invite code — and never exposes one to browser storage (see 4.3).

### 4.2 Should admin access be loopback-only?

**Today:** the two `/v1/admin/*` endpoints require the admin token **and** a loopback caller (`server/internal/admin/guard.go:14-26`). Everything else — `/v1/tools`, `/mcp`, `/v1/inbox/stream` — is bearer-only.

**Options:**

| Option | Effect | Assessment |
| :--- | :--- | :--- |
| **A. No loopback restriction** (status quo for the tool API) | admins work from any host that can reach the port | Consistent with the existing token model; the token itself is the only barrier |
| **B. Loopback-only for admin space operations** | admins read/write only from the server host | Mirrors `/v1/admin/*`; but behind a reverse proxy **every request already looks like loopback**, so the guard protects nothing there (documented in `spec/README.md`, "Local instance via a state file") — which is exactly where a website runs |
| **C. Separate credential** — a distinct operator token for the web GUI | the browser never holds the mint-admin/shutdown credential | The admin role already gives every operator their own minted admin (3.2), which is the same separation of duties at finer granularity: the browser holds *that operator's* admin token, not the instance token |

**Decided (PO ruling): Option A for the transports; the web GUI gets its own listen address.**

- The token model of the bus is "bearer token = full authority" everywhere else; a network-position check only for admins would be inconsistent and, as the reverse-proxy note shows, easily defeated.
- The web GUI is the place where a token is most exposed (browser, XSS surface). It therefore runs on its **own listener**, `AMB_WEB_ADDR` (`host:port`, any host, any port), separate from the API's `AMB_ADDR`. The operator decides the exposure by choosing that address; there is no separate "enable" switch and no loopback check. **Default: `127.0.0.1:8081`** (loopback, the API default port + 1), so a fresh install keeps the GUI off the network. An empty value or `off` disables the GUI. The API listener never serves `/web/*`. Docker images set `AMB_ADDR=:8080` (`server/DEPLOYMENT.md:§6`); the deployment guide tells operators to set `AMB_WEB_ADDR` explicitly there (e.g. `0.0.0.0:8081` to publish it, or `off`) and states that a non-loopback GUI address exposes admin sign-in to the network. Specified in `gui-access.feature`, rule "The web GUI has its own configurable listen address".
- The operator's own mitigation is a reverse proxy with TLS and an authenticating layer in front, or an SSH tunnel — the pattern the deployment guide already recommends (`server/DEPLOYMENT.md:§8`).

**Accepted risk (PO ruling 3):** the web GUI authenticates with an admin token, so the browser session holds a credential with leader rights in every space. The mitigations are the ones in 4.3 — the token never reaches browser storage, the session is re-validated on every request, rotation and revocation kill open sessions, and the GUI listens on loopback (`127.0.0.1:8081`) unless the operator chooses another `AMB_WEB_ADDR`. The residual risk is that a browser-side compromise of the web GUI yields that operator's admin power; with the one-admin-per-operator recommendation (3.2) the damage is bounded to one operator, and a separate operator credential remains the natural follow-up if the web GUI is exposed beyond the host.

### 4.3 GUI-specific security (restated for the admin role)

- Token entered once, sent over the login POST, stored server-side in the session store, never written into HTML, a URL, a log line or a response body.
- Session cookie: `HttpOnly`, `SameSite=Strict`, `Secure` over TLS, `Path=/`, short expiry, cleared on logout.
- Session re-validated against the token on **every** request, so a rotation or a revoke ends open sessions immediately.
- Login returns the same 401 for unknown and malformed tokens and for a non-Bearer scheme, and repeated failures are rate limited. Leader and member tokens may sign in with their normal reach (`gui-access.feature`); only the admin area needs an admin.
- No token from the query string (it lands in logs and `Referer`).
- Phase 1 has no mutating route → no CSRF surface. Phase 2 adds a per-session CSRF token on every mutation, including the admin mint and revoke.
- `html/template` auto-escaping for all agent-controlled text (participant names, admin names, titles, post bodies); CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store` on pages with space content.
- The web GUI shows structure, blackboard posts and **the signed-in admin's own inbox**. It does **not** show another agent's inbox, and it never copies topic messages.
- The web GUI never opens an inbox stream, so it cannot take an admin's or an agent's stream slot and 409 their session.
- Logging: sign-in/sign-out and 401s, never the token; one line per GUI mutation (PO-1) and per admin mint and revoke.

---

## 5. Files delivered

| File | Covers |
| :--- | :--- |
| `gui-access.feature` | Signing in with an admin token, the session identifying the operator as **admin `<name>`**, session cookie, re-validation (rotation kills bootstrap sessions, revocation kills a minted admin's), logout, the GUI's own listen address `AMB_WEB_ADDR` (default `127.0.0.1:8081`, `off` disables), read-only in Phase 1 |
| `gui-admins.feature` | The administration area: listing admins, **minting** an admin (token shown once), revoking a minted admin, the bootstrap admin being un-revocable, admin-only access |
| `gui-inbox.feature` | The signed-in admin's own inbox, read-only: no acknowledge, no stream slot, never another admin's inbox |
| `live-updates.feature` | The GUI polls `GET /web/api/state`; the GUI never takes an inbox stream slot (of an agent or of an admin) |
| `gui-mutations.feature` | Phase 2 space mutations: create invite, **revoke a member or a leader**, post to a thread, send to a topic, **delete a space** — all with CSRF and a confirmation where destructive, all attributed to the admin's name |
| `spaces-and-participants.feature` | Space list with an `admins` column, participants as admins/leader/member, admins listed first and only if they joined |
| `topics.feature` | Open topics of a space, `all` first, closed topics hidden, no inbox content copied |
| `blackboards.feature` | Blackboards, threads, posts of a space |
| `invites.feature` | Pending invites of every space (name + expiry, never the code), redeemed/expired/revoked invites |
| `admin-superuser.feature` | **Superseded** — the `@admin` superuser design of rounds 2 and 3; kept only as a pointer to `docs/design/specs/admin/` |
| `analysis.md` | This file |

---

## 6. Open PO questions

**Closed** (recorded so the reasoning is not lost):

- ~~**Separate operator credential.**~~ **Closed by PO ruling 3:** the web GUI uses an admin token; the risk is accepted and recorded in 4.2. With the admin role the separation of duties is finer-grained: each operator holds *its own* minted admin (3.2).
- ~~**Reserved name / the `@admin` superuser.**~~ **Closed by the round-4 ruling:** admins are named agents of kind `admin`; the bootstrap admin is `admin`. The `@admin` design and its migration work are superseded — see §2 and [`docs/design/specs/admin/`](../admin/).
- ~~**The admin has no inbox.**~~ **Closed:** admins have their own inbox ([`docs/design/specs/admin/admin-messaging.feature`](../admin/admin-messaging.feature)). The web GUI shows the **signed-in** admin's inbox, read-only (3.10).
- ~~**Role of the synthetic membership.**~~ **Closed by the admin role:** the membership seam is `Membership.Admin` + `Membership.Leads()` + `Membership.Joined()` (admin analysis §4.1, seam B), not a synthetic `leader`.
- ~~**Superuser and space lifecycle.**~~ **Closed:** admins create, join and leave spaces with the existing tools; joining is what makes them reachable (admin analysis D7–D9).
- ~~**Reading other agents' inboxes.**~~ **Still out of scope, now settled by the admin role:** an admin reads only its own inbox (admin Q10). The web GUI follows: only the signed-in admin's inbox (3.10).
- ~~**`bus_whoami` for the superuser.**~~ **Closed:** an admin reports its name and the spaces it joined (admin analysis D7, admin-messaging.feature).
- ~~**MCP token identity.**~~ **Closed:** `admin:<agent id>` ([`docs/design/specs/admin/analysis.md`](../admin/analysis.md) §8).
- ~~**Schema reset / migration shape.**~~ **Closed by the admin role:** one fresh `0001_schema.up.sql` with `admin` in `agents.kind` and `participants.role`. Old databases are not detected; operators delete them by hand (admin analysis §7). Any GUI table (e.g. an audit table, Q4) would be a numbered migration after `0001_schema`.
- ~~**Does the web GUI mint admins?**~~ **Closed by PO ruling (round 5): the GUI may mint admins.** The administration area does list, mint and revoke (3.9, `gui-admins.feature`).
- ~~**Admin power: revoke leaders, delete any space.**~~ **Closed by PO ruling (round 5): admins are a strict superset of leaders.** The GUI's mutations follow: it revokes members *and* leaders, and it may delete any space (3.8, `gui-mutations.feature`).
- ~~**Console vs. GUI.**~~ **Closed by PO ruling (round 5): the product is a GUI, not a console.** The product is renamed throughout, and the seam is reassessed in 3.3–3.6: an embedded SPA in strict TypeScript, built with bun, embedded via `go:embed`, no runtime framework.

**Decided in round 6:** Q1 and Q2 (PO-10: option (b), an embedded strict-TS SPA, bun build before `go build`, committed placeholder `dist/`), Q4 (PO-1: one log line per GUI change, no table, no list), Q7 (PO-4: delete a space behind a confirmation naming it), and invite cancel (PO-3: GUI-only endpoint by invite id, no tool).

**Still open:**

3. ~~**Loopback rule for admin space access.**~~ *Decided (PO):* no network rule on the tool API; the GUI's exposure is set by `AMB_WEB_ADDR` (4.2).
5. **The admin inbox page: read-only, or also acknowledge?** Recommendation: read-only (3.10) — the operator's omp session may be streaming the same inbox, and acknowledging would delete entries out from under it. Confirm.
6. **Live updates: polling or SSE?** Recommendation: start with polling `GET /web/api/state` every 5 s, and add `GET /web/api/events` (an SSE of change hints) only if the polling cost shows up. It needs a new broker topic family, because the existing one is per-agent inbox delivery (3.5). Confirm.
8. **Richer admin metadata.** `bus_list_admins` returns name/bootstrap/tokens/spaces. Should the GUI also show when an admin last acted? That needs a new field.
9. **GUI path.** `/web` versus `/ui` versus `/` on the GUI listener? (The default address is decided: `AMB_WEB_ADDR=127.0.0.1:8081`.)
10. **Where the specs live.** `server/spec/web.feature` plus one file per page, or a `server/spec/web/` folder? And `spec/README.md:90` ("no UI") must be updated either way.
