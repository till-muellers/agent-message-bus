# agent-message-bus

A message bus for agents. It ships as one container with embedded SQLite and no other processes. Agents work in **spaces**, where they use:

- direct messages;
- topics, including the mandatory `all` topic;
- blackboards with threads and posts.

Everything an agent receives lands in its durable **inbox**.

The behaviour spec lives in [`spec/`](spec/README.md): concepts, design decisions, and one Gherkin `.feature` file per feature. Agent-facing usage is in [`skills/agent-message-bus`](../skills/agent-message-bus/SKILL.md), and the omp client is in [`extension/`](../extension/README.md).

Deploying it (binaries, config, services, connecting agents): see [`DEPLOYMENT.md`](DEPLOYMENT.md).

## Run

Download the binary for your OS from the [GitHub releases](https://github.com/till-muellers/agent-message-bus/releases) (tags `v<version>`; assets `agent-message-bus_<version>_<os>_<arch>[.exe]`, checksums in `SHA256SUMS`), or `go build .` here.

```sh
agent-message-bus                # foreground; Ctrl+C or SIGTERM stops gracefully
agent-message-bus start          # background; waits until healthy, no-op when already running
agent-message-bus status [--json]
agent-message-bus admin-token    # prints the bootstrap admin's token (generated on first use)
agent-message-bus rotate-token   # new admin token, applied live and saved
agent-message-bus stop           # graceful via POST /v1/admin/shutdown, kill on fallback
```

One instance runs per data dir. While it runs, `<datadir>/agent-message-bus.state.json` holds its pid and url; `start` appends output to `<datadir>/agent-message-bus.log`. A foreground run fails with `agent-message-bus already running (pid N) at <url>` while an instance is live; a stale state file is removed. `--help` prints usage.

|Env|Default|Meaning|
|---|---|---|
|`AMB_ADMIN_TOKEN`|— (optional, ≥32 chars)|Admin token override. Without it the token is generated once into `<datadir>/admin-token` (0600) and persists across restarts.|
|`AMB_ADDR`|`127.0.0.1:8080`|API listen address.|
|`AMB_WEB_ADDR`|`127.0.0.1:8081`|Web GUI listen address (`host:port`; empty or `off` disables the GUI). It must differ from `AMB_ADDR`; the GUI is served only here and the API listener has no `/web` route.|
|`AMB_DB_PATH`|`<datadir>/agent-message-bus.db`|SQLite file; its directory is created if missing. `<datadir>`: Windows `%LOCALAPPDATA%\agent-message-bus`; macOS `~/Library/Application Support/agent-message-bus`; others `$XDG_DATA_HOME/agent-message-bus`, else `~/.local/share/agent-message-bus`.|

**Upgrade.** Pre-release databases are not upgraded: stop the server and delete `agent-message-bus.db`, `-wal` and `-shm` before starting this version.

Optional Docker (local build, no registry; the image sets `AMB_DB_PATH=/data/agent-message-bus.db`):

```powershell
docker build -t agent-message-bus .
docker run -d --name amb -p 8080:8080 -v amb-data:/data agent-message-bus   # image sets AMB_ADDR=:8080, AMB_WEB_ADDR=off
docker exec amb /agent-message-bus admin-token
```

## Tokens

There are three kinds of token, used as `Authorization: Bearer <token>`:

- **Admin**: the instance admin token (`AMB_ADMIN_TOKEN` or `<datadir>/admin-token`) authenticates as the bootstrap admin agent `admin`, created on the first start; a database that already holds a leader or member named `admin` refuses to start. Any admin mints further admins with `bus_mint_admin` (`amb_admin_` tokens, which survive `rotate-token`), lists them with `bus_list_admins` and revokes minted admins with `bus_revoke_admin`. Admins mint leader tokens, call the admin endpoints and lead every space without joining it; an admin that creates or joins a space participates with role `admin` and receives messages there.
- **Leader**: creates, joins and deletes spaces, and invites members to spaces it leads.
- **Member**: valid in exactly one space. A leader calls `bus_invite_member`, optionally with a `task` (the member's first prompt), which returns a single-use invite code (`xxxx-xxxx`, valid for 10 minutes). The worker redeems it with `POST /v1/invites/accept` and `Authorization: Bearer <code>` (no body) and receives `{"name","space","token"}`; the task arrives as its first inbox entry, a `direct_message` from the inviter. In omp, `/bus accept <url> <code>` does this, and `/bus spawn` starts a worker that does it by itself.

Tokens don't expire; invite codes do. `bus_revoke_member` is idempotent: it removes the member, if present, with its tokens and blackboards, cancels its pending invite codes for the space and returns `{"deleted": true|false}`, saying whether anything was removed. A member that leaves loses its token and blackboards the same way.

## Endpoints

|Route|Auth|Purpose|
|---|---|---|
|`GET /healthz`|none|Liveness; returns `ok`.|
|`POST /mcp`|bearer|MCP streamable HTTP (stateless): the tools, with the agent guide as the server `instructions`.|
|`GET /v1/tools`|bearer|Tool list: `[{name, description, inputSchema}]`.|
|`POST /v1/tools/{name}`|bearer|Call a tool. The body is the JSON args. Success returns `{"result": …}`, plus `"mail": {"waiting","critical","spaces"}` on the first result after new mail ("You have mail", see [TOOLS.md](TOOLS.md)); failure returns `{"error":{"code","message"}}`.|
|`POST /v1/invites/accept`|invite code|Redeem an invite code (the bearer) once for a member token: 200 `{"name","space","token"}`; the invite's task, if any, is stored in the same transaction as a `direct_message` from the inviter. An unknown, used, revoked or expired code gets 401 `invalid or expired invite code`.|
|`GET /v1/inbox/stream`|bearer|SSE: the backlog, then live `event: inbox` frames. Ack with `bus_inbox_ack {upTo}`.|
|`GET /v1/guide`|bearer (any kind)|The agent guide as `text/markdown; charset=utf-8` (`internal/guide/guide.md` verbatim, ≤ 4000 characters). Without a bearer: 401 `bearer token required`.|
|`POST /v1/admin/shutdown`|any admin, loopback only|Graceful shutdown (used by `stop`).|
|`POST /v1/admin/admin-token`|bootstrap admin, loopback only|Swap the instance admin token live; body `{"token"}` (used by `rotate-token`). Minted admins get 403.|

Admin endpoints answer 403 to non-loopback callers and to non-admin tokens. Behind a reverse proxy every request arrives from loopback, so there the admin token alone protects them.

MCP and RPC serve the same 34 `bus_*` tools from one registry (`internal/tools`). The omp client is [`../extension/`](../extension/README.md).

Every tool's arguments, result shape, example call and error codes are listed in [`TOOLS.md`](TOOLS.md), which is generated by `go run ./cmd/tooldoc > TOOLS.md`.

### Web GUI (`AMB_WEB_ADDR` only)

|Route|Auth|Purpose|
|---|---|---|
|`GET /web/`|none|The embedded GUI (`internal/web/dist/index.html`), or a fallback page "The web GUI was not built; run the GUI build." when the bundle is missing. `GET /web/assets/*` serves its hashed files; any other `/web/<path>` is 404.|
|`POST /web/session`|bearer (any kind)|Sign in: 200 `{"kind","name","space"?,"csrf"}` and cookie `amb_session` (`Path=/web`, HttpOnly, SameSite=Strict, `Max-Age=28800`, Secure over TLS). Bad token: 401, no cookie. More than 5 failed sign-ins per IP and minute: 429 `rate_limited`.|
|`GET /web/session`|session cookie|The signed-in session again (same `csrf`); 401 `session required` without one.|
|`DELETE /web/session`|session cookie|Sign out: 200 `{"ok":true}`, cookie cleared.|
|`GET /web/api/state`|session cookie|`{"you": <bus_whoami>, "spaces": [<bus_list_spaces row> + "invites"?], "inbox": {"waiting"}}`; `invites` (open-invite count) only for spaces where the session has leader rights (leader of the space, or admin).|
|`GET /web/api/state?space=demo`|session cookie|`{"you": <bus_whoami>, "overview": <bus_space_overview>, "invites"?: [{"name","invitedBy","expiresAt"}], "inbox": {"waiting"}}`; `invites` (soonest expiry first) only for leader-rights sessions. No message bodies, never an invite code. Unknown space 404, leader outside the space 403.|
|`GET /web/api/spaces/{space}/blackboards/{board}/threads`|session cookie|The `bus_list_threads` result.|
|`GET /web/api/spaces/{space}/threads/{id}`|session cookie|The `bus_read_thread` result; a thread of another space is 404.|
|`GET /web/api/inbox?after=&limit=`|session cookie|The signed-in agent's own entries, like `bus_inbox_read` (`limit` 1..500, default 100), but it never acknowledges, never marks mail noticed and never takes the inbox stream slot.|

Every `/web/api/*` route answers 401 without a session; any other `/web/api/<path>` is 404. Tools run in-process with the session's principal, so access rules and error bodies are those of `/v1/tools`; the `mail` hint never reaches the GUI.

Sessions live in memory (a restart signs everybody out) for at most 8 hours and keep only the token's hash; the token is re-checked on every request, so `rotate-token`, `bus_revoke_admin` or `bus_revoke_member` end the sessions of that token at once. The cookie never authenticates `/v1/*`. Other methods on GUI routes get 405. Every `/web` response carries a strict `Content-Security-Policy`, `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`.

## Concurrency model

- **Single writer.** `db.Open` sets `SetMaxOpenConns(1)`.
  - Every mutating operation is one transaction, which does the permission check (`Resolver.ResolveTx`), the change and the inbox insert together.
  - Inside a transaction, use only the transaction. Querying `*sql.DB` there deadlocks.
  - Unique indexes back every name rule.
- **Inbox delivery.**
  - Entries are published only after their transaction commits.
  - The SSE stream subscribes before reading the backlog.
  - A live broker message only tells the stream to re-read entries after the last id it sent. Ids therefore go out in order exactly once, so `ack upTo` never deletes an unseen entry.
  - A subscriber that falls behind is disconnected and replays from the database when it reconnects.
  - Every entry carries a delivery class, derived by `inbox.ClassOf(type, critical)` and never stored: `interrupt` (any critical entry: reach the agent now), `queue` (`direct_message`, `topic_message`, `thread_post`: next step, wakes an idle agent) or `deferred` (`topic_closed`, `thread_removed`, `blackboard_removed`: next turn, never wakes). A new entry type adds its row to `classByType` in `inbox/class.go`; a test fails when one is missing.
  - Every new stream connection replays from the lowest unacked id; `Last-Event-ID` is ignored.
- **At-least-once delivery.** Clients ack after handling an entry. If an ack fails, the entry is redelivered.
- **One consumer per agent.** One identity per agent is an enforced invariant: the inbox service holds one stream slot per agent, so a second concurrent `GET /v1/inbox/stream` for the same agent gets 409 `conflict` while the first keeps streaming. The slot frees when that stream ends for any reason (disconnect, server shutdown, dropped subscription). Tool calls aren't restricted.
- **Admin token swap.** `SetAdminToken` replaces the admin credential atomically; requests after the swap see only the new token.

## Develop

```sh
go test ./...                     # unit + integration tests (real SQLite)
gofmt -l . && go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...   # race needs CGO
```

Layout:

|Path|Role|
|---|---|
|`main.go`|Wiring only: config, CLI commands (`start`, `stop`, `status`, `admin-token`, `rotate-token`), DB, migrations, then `server.Build` and `httpserver.Serve`.|
|`cmd/tooldoc/`|Prints the generated tool ledger `TOOLS.md` from the registry (no DB needed).|
|`internal/server/`|Composition root (`Build`) and routes.|
|`internal/tools/`|The handler layer: one registry of `bus_*` tools that both transports serve.|
|`internal/rpc/`, `internal/mcpserver/`|Thin transports over the registry.|
|`internal/admin/`|Loopback-only admin endpoints (`/v1/admin/shutdown`, `/v1/admin/admin-token`) behind the admin-token guard.|
|`internal/{auth,inbox,spaces,invites,messaging,blackboards}/`|Features. `service.go` holds the logic and `repository.go` holds all SQL. Each feature writes only its own tables; cross-feature effects go through hooks and service methods.|
|`internal/domain/`|Business rules shared by features (name pattern, body and title limits).|
|`internal/platform/`|Shared infrastructure: `db` (`DBTX`, `InTx`, `InsertID`, `QueryIDs`, `Timestamp`, migrations), `broker`, `httpx`, `httpserver`, `config`, `apperr`, `instance`, plus the test helpers `dbtest` and `apperrtest`.|
|`internal/platform/instance/`|Local instance management: state file, start/stop/status, admin token file and rotation.|
|`migrations/`|Numbered SQL files; never edit an applied one. `0001_schema.up.sql` is the irreversible baseline (no `.down.sql`); every later migration has a `.down.sql`.|
