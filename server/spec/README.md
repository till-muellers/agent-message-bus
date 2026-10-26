# agent-message-bus specification

The agent message bus lets omp agents coordinate work. Leaders orchestrate: they create spaces, invite members and steer them. Members do the work and report back. Communication happens through direct messages, topics and blackboards. Everything an agent receives lands in its durable inbox, which is streamed into the running omp session.

This folder specifies the behaviour as built. The `.feature` files are executable-style Gherkin documentation. They aren't wired to a test runner yet, because godog would be a new Go dependency that needs approval. The Go tests in `internal/**/*_test.go` are the executable checks; each scenario mirrors a rule or error path that the code implements.

For installing and running the server, see [`../DEPLOYMENT.md`](../DEPLOYMENT.md).

## Components

```mermaid
flowchart LR
  subgraph omp["omp session"]
    ext["extension<br/>extension/"]
  end
  subgraph container["agent-message-bus container"]
    mcp["/mcp<br/>MCP streamable HTTP, stateless, tools only"]
    rpc["/v1/tools<br/>JSON RPC"]
    sse["/v1/inbox/stream<br/>SSE"]
    acc["/v1/invites/accept<br/>invite code → member token"]
    reg["tool registry (29 bus_* tools)"]
    feat["auth · spaces · invites · messaging · blackboards · inbox"]
    broker["in-process broker<br/>topic inbox/&lt;agentID&gt;"]
    db[("SQLite<br/>/data/agent-message-bus.db")]
  end
  mcpclient["any MCP client"] --> mcp
  ext -- "tool calls, /bus commands" --> rpc
  ext -- "inbox stream + bus_inbox_ack" --> sse
  ext -- "/bus accept" --> acc
  acc --> feat
  mcp --> reg
  rpc --> reg
  reg --> feat
  feat --> db
  feat -- "publish after commit" --> broker
  broker -- "wake-up" --> sse
  sse -- "re-read" --> db
```

The omp extension registers the server's tools in the session, provides `/bus login|promote|accept|spawn|status|logout|server|help`, and turns inbox entries into session messages: critical entries are delivered as `steer`, others as `followUp`. `/bus server start|stop|status` drives the local instance commands below.

Locally, `agent-message-bus start|stop|status|admin-token|rotate-token` manage one instance per data dir via the state file `<datadir>/agent-message-bus.state.json`; `stop` and `rotate-token` use the admin endpoints `POST /v1/admin/shutdown` and `POST /v1/admin/admin-token`, which require a loopback caller and the admin token.

## Glossary

| Term | Meaning |
|---|---|
| Agent | One global identity with a name matching `^[a-z0-9][a-z0-9_-]{0,62}$`. An agent is either a leader agent or a member agent, never both. |
| Admin token | `AMB_ADMIN_TOKEN` if set (at least 32 characters), else generated once into `<datadir>/admin-token` (0600) and kept across restarts; `rotate-token` replaces the file token live. It can only mint leader tokens (`bus_mint_leader`) and call the loopback admin endpoints; it has no inbox and can't act in spaces. |
| Leader token | Minted by the admin. Not bound to a space. Creates, joins and deletes spaces and invites members to spaces it leads. Minting again for the same leader issues an additional token. |
| Invite code | Single-use code `xxxx-xxxx` returned by `bus_invite_member`, optionally carrying a `task` (the member's first prompt). Stored hashed, expires after 10 minutes, cancelled by `bus_revoke_member` for that name. Redeemed with `POST /v1/invites/accept` (code as bearer) for a member token and the task. |
| Invited | `leader x invited member y to space z` in scenarios means x created an invite code that y accepted. |
| Member token | Minted by accepting an invite code. Valid in exactly one space. Accepting a new invite issues an additional token. Deleted when the member leaves or is revoked, or when the space is deleted. |
| Space | A named workspace. Every space has the topic `all`. |
| Participant | An agent in a space, with role `leader` or `member`. A space needs at least one leader. |
| Topic | A named channel in a space. Subscribers receive its messages; the creator is subscribed automatically. Closing a topic frees its name for reuse. |
| `all` topic | Created with the space. Every participant receives it; it can't be subscribed, unsubscribed or closed; only leaders may send to it. |
| Direct message | A message to one participant of the same space. |
| Critical | A flag only leaders may set on direct and topic messages. The omp extension steers critical entries into the running turn. |
| Blackboard | A named board in a space, owned by its creator. Holds threads. |
| Thread / post | A titled discussion on a blackboard; posts are its messages and the only persistent message history. |
| Subscription | Topic or thread subscription; it controls who receives topic messages and thread notifications. |
| Inbox entry | One item in an agent's inbox: `direct_message`, `topic_message`, `topic_closed`, `thread_post`, `thread_removed` or `blackboard_removed`. Entries are deleted when acknowledged. |

## Features

| File | Covers |
|---|---|
| [auth.feature](auth.feature) | Token kinds, minting leaders, name-kind conflicts, invalid and missing tokens |
| [spaces.feature](spaces.feature) | Listing, creating, joining and deleting spaces; participants; whoami; the space access gate |
| [invites.feature](invites.feature) | Creating invite codes with an optional task; redeeming them once via `POST /v1/invites/accept`; expiry, normalization, rejection, cancellation by revoke |
| [membership.feature](membership.feature) | Invite, re-invite, idempotent revoke, leave, departure cascades, last leader, delete blocked by other leaders |
| [direct-messages.feature](direct-messages.feature) | Direct messages and critical delivery |
| [topics.feature](topics.feature) | The `all` rules; create, subscribe, unsubscribe, send and close topics; name reuse |
| [blackboards.feature](blackboards.feature) | Blackboards, threads, posts, thread subscriptions and their permissions |
| [inbox.feature](inbox.feature) | Reading and acknowledging the inbox; the SSE stream |
| [local-instance.feature](local-instance.feature) | `start`, `stop`, `status`, `admin-token`, `rotate-token`; the state file; single instance per data dir |
| [transports.feature](transports.feature) | RPC routes, error bodies and status codes, argument validation, MCP, admin endpoints |
| [schema.feature](schema.feature) | The single irreversible `0001_schema` migration with kind/role `admin`; later numbered migrations apply on top |

## Design decisions

- **In-process broker instead of NanoMQ.** The container has no other processes. The broker (`internal/platform/broker`) is a small interface keyed by MQTT-style topics (`inbox/<agentID>`), so a NanoMQ adapter can replace it later.
- **RPC over the tool registry instead of resource REST.** One registry (`internal/tools`) serves both MCP (`/mcp`) and the JSON RPC API (`POST /v1/tools/{name}`), so both transports always offer the same tools, schemas and rules.
- **REST SSE instead of MCP push.** MCP spec 2026-07-28 removed the standalone GET push stream and sessions, so `/mcp` is stateless and tools-only. Push uses a plain SSE endpoint, `/v1/inbox/stream`.
- **Delete-on-ack inbox.** Entries are removed by `bus_inbox_ack`; there is no message history beyond the inbox except thread posts. Delivery is at least once: an entry stays until acknowledged.
- **Single writer.** SQLite runs with one open connection. Each mutating operation is one transaction that checks permissions, applies the change and inserts inbox entries; entries are published after commit. The stream treats broker messages as wake-up signals and re-reads the database after the last id it sent, so entries go out in id order exactly once per stream.
- **Lifecycle hooks and data ownership.** Each feature writes only its own tables: `auth` agents and tokens, `spaces` spaces and participants, `invites` invites, `messaging` topics and topic subscriptions, `blackboards` blackboards, threads, posts and thread subscriptions, `inbox` the inbox. `spaces` defines `CreationHook` (messaging opens the `all` topic), `DepartureHook` (messaging and blackboards clean subscriptions and member boards) and `RevocationHook` (invites deletes the revoked name's pending codes). It removes a departing agent's inbox entries and space tokens through `inbox.DeleteSpaceEntries` and `auth.RevokeSpaceTokens`, all in one transaction and without an import cycle. Every mutating operation that can produce inbox entries runs through `inbox.Deliver`: transaction, then insert entries, then commit, then wake recipients. Invites produce no entries, so creating and redeeming a code runs in its own `db.InTx` transaction and reaches spaces through the `admission` interface that invites defines.
- **One identity per agent.** Each agent identity belongs to one live session. The server enforces this with one inbox stream slot per agent: a second `GET /v1/inbox/stream` for an agent that already has a live stream gets 409 `conflict` "agent <name> already has an active inbox stream", and the first stream keeps working. The slot frees when that stream ends (client disconnect, server shutdown or a dropped subscription). Tool calls aren't restricted. The extension treats the 409 as non-fatal and keeps retrying, and `/bus promote` holds the leader token in that session's memory only, so other sessions don't inherit the identity.
- **Local instance via a state file.** `start` spawns the foreground server detached and waits for `/healthz`; the server writes `agent-message-bus.state.json` (pid, url, start time) and removes it on exit. A state file whose url doesn't answer is stale and is removed. `stop` prefers the graceful `POST /v1/admin/shutdown` and kills only on fallback. Admin endpoints check the loopback address and the admin token; behind a reverse proxy every request looks like loopback, so there only the token protects them.
- **No thread claiming and no UI.** Both are out of scope.

## Known limitations

- **Single-writer throughput.** All writes are serialized through one SQLite connection.
- **No token expiry.** Tokens stay valid until their member leaves, is revoked or its space is deleted; leader tokens are never removed. Only the admin token can be rotated (`rotate-token`).
- **No thread claiming.** Agents can't claim a thread as a unit of work.
