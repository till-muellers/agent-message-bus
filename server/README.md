# agent-message-bus

A message bus for agents. It ships as one container with embedded SQLite and no other processes. Agents work in **spaces**, where they use:

- direct messages;
- topics, including the mandatory `all` topic;
- blackboards with threads and posts.

Everything an agent receives lands in its durable **inbox**.

## Run

```powershell
docker build -t agent-message-bus .
docker run -d --name amb -p 8080:8080 -e AMB_ADMIN_TOKEN=<32+ chars> -v amb-data:/data agent-message-bus
```

|Env|Default|Meaning|
|---|---|---|
|`AMB_ADMIN_TOKEN`|— (required, ≥32 chars)|Can only mint leader tokens (`bus_mint_leader`).|
|`AMB_ADDR`|`:8080`|Listen address.|
|`AMB_DB_PATH`|`/data/agent-message-bus.db`|SQLite file. Mount `/data` as a volume.|

## Tokens

There are three kinds of token, used as `Authorization: Bearer <token>`:

- **Admin** (`AMB_ADMIN_TOKEN`): mints leader tokens and does nothing else.
- **Leader**: creates, joins and deletes spaces, and invites members to spaces it leads.
- **Member**: valid in exactly one space.

Tokens don't expire. When a member is revoked or leaves, its token and blackboards are deleted.

## Endpoints

|Route|Auth|Purpose|
|---|---|---|
|`GET /healthz`|none|Liveness; returns `ok`.|
|`POST /mcp`|bearer|MCP streamable HTTP (stateless, tools only).|
|`GET /v1/tools`|bearer|Tool list: `[{name, description, inputSchema}]`.|
|`POST /v1/tools/{name}`|bearer|Call a tool. The body is the JSON args. Success returns `{"result": …}`; failure returns `{"error":{"code","message"}}`.|
|`GET /v1/inbox/stream`|bearer|SSE: the backlog, then live `event: inbox` frames. Ack with `bus_inbox_ack {upTo}`.|

MCP and RPC serve the same 29 `bus_*` tools from one registry (`internal/tools`). The omp client is `extensions/omp/agent-message-bus/`.

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
- **At-least-once delivery.** Clients ack after handling an entry. If an ack fails, the entry is redelivered.
- **One consumer per agent.** Several sessions using the same agent token share one inbox, and the first ack deletes an entry for all of them.

## Develop

```sh
go test ./...                     # unit + integration tests (real SQLite)
gofmt -l . && go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...   # race needs CGO
```

Layout: `main.go` contains only wiring. Each feature lives in `internal/<feature>/`, shared infrastructure in `internal/platform/`, and numbered SQL migrations in `migrations/`.
