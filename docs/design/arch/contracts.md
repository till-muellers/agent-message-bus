# Contracts — every client-visible server contract

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix (e.g. `mail/bus-wait.feature`) are relative to `docs/design/specs/`.

Owner: **Arch-server** (shared with Arch-client; change requests go to Arch-server). Status per item: **Fixed** (spec-verbatim, PO-decided, or decided here), **[AD-n]** (architect decision, `server.md` §8).
Slice ids are `docs/design/specs/REVIEW.md` §11 numbers; splits carry a letter; `D1` is the new delivery-class slice; `0` commits the design (`slices.md`). Client slice ids (E*, P*, GUI-*, PL-*) are in `slices-client.md`; the combined order is `BACKLOG.md`. Every client contract request and its decision is in §12.

PO decisions applied (2026-10-26): PO-1 one log line per GUI change; PO-2 tiers approved; **PO-3 no `bus_cancel_invite` tool — invite cancel is a web endpoint with an opaque invite id (§10.6)**; PO-4 GUI deletes spaces after confirmation; **PO-5 lifecycle notices wait for the next turn, via a delivery class on every entry (§4.1)**; PO-6 omp only, no upstream-pi fallback (client); PO-7/8/9 approved; PO-10 (b) approved; PO-11 S1 (client).

---

## 0. Conventions (unchanged unless stated)

| Item | Contract | Anchor |
|---|---|---|
| RPC call | `POST /v1/tools/{name}`, body = JSON args, `Authorization: Bearer <token>` | `rpc/handler.go:40-63` |
| RPC success | `200 {"result": <tool result>}` — plus optional sibling `"mail"` from 13b (§9) | `rpc/handler.go:62` |
| RPC/HTTP error | `{"error":{"code":"<kind>","message":"<msg>"}}`, status by kind | `platform/httpx/json.go:13-50` |
| Error kinds | `invalid` 400, `unauthenticated` 401, `forbidden` 403, `not_found` 404, `conflict` 409, `internal` 500 (`"internal error"`). **No new `apperr` kind.** The one exception on the wire is the web sign-in limiter's local `429 rate_limited` (§10.1, [AD-9], Q-A9), written by `internal/web` only. | `platform/apperr/apperr.go:13-19` |
| MCP error | tool result `isError: true`, text `<code>: <message>` | `mcpserver/server.go:49-58` |
| MCP success | one `TextContent` = JSON; `structuredContent` when the result is an object | `mcpserver/server.go:60-70` |
| Args | closed schema (`additionalProperties:false`); unknown field → `invalid "invalid arguments: …"`; missing → `invalid "<field> is required"` | `tools/decode.go:13-42`, `tools/schema.go:10-23` |
| Names | `^[a-z0-9][a-z0-9_-]{0,62}$`; error `invalid "<field> must match …"` | `domain/text.go:13,22-28` |
| Timestamps | RFC3339Nano UTC strings | `platform/db/dbtx.go:35` |

---

## 1. Tool registry by slice

| After slice | Count | Added |
|---|---|---|
| today | 29 | — |
| 6a | 31 | `bus_mint_admin`, `bus_list_admins` |
| 6b | 32 | `bus_revoke_admin` |
| 8 | 33 | `bus_space_overview` |
| 9 | 34 | `bus_wait` |

Final: **34 server tools** (PO-3: no `bus_cancel_invite`). `bus_spawn_worker` is extension-local; it appears only in the TOOLS.md "omp extension" section (slice 3). Each slice sets the absolute count in `tools/registry_test.go:11`, `rpc/handler_test.go:159`, `spec/transports.feature:54-57,191`, `spec/README.md:21`, `server/README.md:69`. (REVIEW plans 32 at slice 6; the split into 6a/6b passes through 31.)

---

## 2. New tools

### 2.1 `bus_mint_admin` [6a] — Fixed (`admin/admin-tokens.feature:65-141`, `admin/analysis.md:217`)

- Group `Admin`. Access: `Admins only.`
- Args: `{"name": string}` required.
- Result: `{"name": "ops", "token": "amb_admin_<43 base64url>"}`.
- Order of checks: admin → name → kind.
- Errors: `forbidden "admin token required"`; `invalid "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"`; `conflict "admin is the bootstrap admin; its token is the instance admin token"`; `conflict "agent lead is a leader agent"` / `"agent worker is a member agent"`.
- Effects: agent kind `admin` created or reused; one `tokens` row (`space_id` NULL); re-mint = additional token; no participation. Log `msg="admin minted" by=<caller> name=<target>` after commit.

### 2.2 `bus_list_admins` [6a] — Fixed (`admin-tokens.feature:143-163`)

- Access: `Admins only; no side effects.` Args: none.
- Result: `[{"name": "admin", "bootstrap": true, "tokens": 0, "spaces": ["demo"]}, …]` sorted by name; `tokens` = minted token rows (0 for the bootstrap admin and revoked admins); revoked admins stay listed.
- Errors: `forbidden "admin token required"`.

### 2.3 `bus_revoke_admin` [6b] — Fixed (`admin-tokens.feature:165-266`, `no-empty-spaces.feature:105-114`)

- Access: `Admins only.` Args: `{"name": string}` required.
- Result: `{"deleted": true|false}` — true if tokens or participations were removed; unknown or already-revoked name → `false` (not an error).
- Errors: `forbidden "admin token required"`; `forbidden "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"`; `conflict "agent lead is a leader agent"`.
- Effects (one tx): delete all its tokens; departure from every joined space (participant row, inbox entries, subscriptions; boards kept); spaces left empty are deleted; agent row kept (re-mint = same agent id); self-revoke allowed. Log `msg="admin revoked" by name deleted` (no-op revokes too; refused calls nothing).

### 2.4 `bus_space_overview` [8] — Fixed (`guidance/space-overview.feature`)

- Group `Spaces & membership`. Access: `Participants and admins; no side effects.`
- Description (verbatim, `space-overview.feature:22-24`): "Shows one space at a glance: you and your role, the participants, open topics with your subscriptions, blackboards and the 10 most recently active threads. Participants and admins; no side effects."
- Args: `{"space": string}` required.
- Result:
```json
{"space": "demo",
 "you": {"name": "worker", "role": "member"},
 "participants": [{"name": "lead", "role": "leader"}],
 "topics": [{"name": "all", "createdBy": "lead", "subscribed": true}],
 "blackboards": [{"name": "plan", "owner": "lead", "threads": 1}],
 "recentThreads": [{"id": 1, "blackboard": "plan", "title": "design", "createdBy": "lead",
                    "posts": 1, "lastPostAt": "2026-…Z", "subscribed": false}]}
```
  `you.role` ∈ `admin|leader|member|null` (null = unjoined admin); participants admins → leaders → members, each by name; topics open only, `all` first; `recentThreads` ≤ 10 across all boards, latest post first; empty lists `[]`. From slice 12 participants carry `connected` (§3.4).
- Errors: `invalid "space is required"`; `not_found "space nope not found"`; `forbidden "token is not valid for space beta"`; `forbidden "not a participant of space demo"`.

### 2.5 `bus_wait` [9] — Fixed (`mail/bus-wait.feature`)

- Group `Inbox`. Access: `Leaders, members and admins.`
- Args (all optional): `timeoutSeconds` integer 1..300 (default 30), `after` integer (default 0), `from` string (sender name), `threadId` integer.
- Result: `[Entry…]` (§4), id order, ≤ 100, `[]` on timeout. RPC `{"result": [...]}`; MCP: text only (array → no `structuredContent`).
- Semantics: returns at once if matching unacked entries with id > `after` exist; else blocks until one arrives in the caller's scope, the timeout, client disconnect, server shutdown (→ `[]`), or a dropped broker subscription (→ re-read once). Never acks. Never takes the stream slot. Counts as `connected` while blocked [12].
- Timeouts (CR-7): the server sets no write or idle timeout on `/v1/tools/*` or `/mcp` (`http.Server` has only `ReadHeaderTimeout: 10s`, `main.go:145`), so a 300 s call is never cut server-side. Slice 9 moves the `http.Server` construction into `internal/server` (`NewHTTPServer`) and pins `WriteTimeout == 0` with a test, so a later timeout cannot silently cut waits. Only graceful shutdown ends a wait early (returns `[]`). Clients use a transport timeout ≥ `timeoutSeconds + 10 s`. A reverse proxy in front must allow ≥ 310 s (`server/DEPLOYMENT.md`, slice 9).
- Registry (CR-9): `GET /v1/tools` lists every tool to every token kind, `bus_wait` (from slice 9) and `bus_inbox_ack` included, with their `inputSchema`; the server never filters the list per client (today's behaviour, `rpc/handler.go:34-37`; slice 9 pins it per token kind).
- Errors: `invalid "timeoutSeconds must be 1..300"`.
- The omp extension registers a local shadow with the same name and schema (`omp-parity/omp-tools.feature:129-188`); it should copy the schema from `GET /v1/tools`.

---

## 3. Changed tools

### 3.1 Description and access (all tools) [3, then 4c/4d/6a]

`Tool.Access` is a separate field; descriptions open with the purpose and contain the access sentence; TOOLS.md "Access" column = `Tool.Access` (`tool-descriptions.feature:13-19`). Pitfall sentences per `tool-descriptions.feature:31-50`; critical property: **"Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true."** (from 4c; slice 3 interim: "Leaders only. omp recipients are interrupted (steer); other clients see critical: true.").

Descriptions state only shipped behaviour. Slice 3 ships interim texts; each later slice adds its clause when its behaviour ships:

| Clause (final text) | Slice 3 interim | Added by |
|---|---|---|
| `bus_inbox_ack`: "acknowledging everything lets the next mail notify again" | — (omitted) | 13b (13a stores the edge; the notify first becomes visible with the hint) |

Access sentences. Spec-verbatim rows marked ✔ (`tool-descriptions.feature:64-69`); the rest [AD-13].

| Tool | Slice 3 (no admin authority yet) | Final (from 4c/4d/6a) |
|---|---|---|
| `bus_mint_leader` | Admin token only. | Admins only. |
| `bus_mint_admin` / `bus_revoke_admin` | — | Admins only. |
| `bus_list_admins` | — | Admins only; no side effects. |
| `bus_whoami` | Any token; no side effects. | Any token; no side effects. |
| `bus_list_spaces` | Leaders see all spaces, members only their own; no side effects. | Leaders and admins see all spaces, members only their own; no side effects. |
| `bus_list_participants`, `bus_list_topics`, `bus_list_blackboards`, `bus_list_threads`, `bus_read_thread` | Participants only; no side effects. | Participants and admins; no side effects. |
| `bus_space_overview` ✔ | — | Participants and admins; no side effects. |
| `bus_create_space`, `bus_join_space` | Leaders only. | Leaders and admins. |
| `bus_leave_space` | Participants only. | Participants. |
| `bus_delete_space` | Leaders of the space, once no other leader remains. | Leaders of the space once no other leader remains, and admins. |
| `bus_invite_member` ✔, `bus_revoke_member` | Leaders of the space. | Leaders of the space and admins. |
| `bus_send_direct` | Participants only; critical requires a leader. | Participants and admins; critical requires a leader or an admin. |
| `bus_send_topic` | Participants only; only leaders may send to all or send critical. | Participants and admins; only leaders and admins may send to all or send critical. |
| `bus_create_topic`, `bus_create_blackboard`, `bus_create_thread`, `bus_add_post` | Participants only. | Participants and admins. |
| `bus_subscribe_topic`, `bus_unsubscribe_topic`, `bus_subscribe_thread`, `bus_unsubscribe_thread` | Participants only. | Participants (admins join first). |
| `bus_close_topic` | The topic's creator or a leader of the space. | The topic's creator, leaders of the space and admins. |
| `bus_delete_blackboard` | The board's owner or a leader of the space. | The board's owner, leaders of the space and admins. |
| `bus_remove_thread` | The thread's creator or a leader of the space. | The thread's creator, leaders of the space and admins. |
| `bus_inbox_read` ✔, `bus_inbox_ack`, `bus_wait` ✔ | Leaders and members. | Leaders, members and admins. |
| `bus_spawn_worker` ✔ (TOOLS.md only) | Leaders of the space. | Leaders of the space and admins. |

`bus_spawn_worker` TOOLS.md entry (slice 3, `tool-descriptions.feature:22-23,73-79`): description starts "Starts a worker agent"; args `space` ("Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$."), `name`, `task` ("Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report.", from slice 7), `cwd`; result `{name, space, launcher}`.

### 3.2 Identity and listing [4a, 4d]

- `bus_whoami` → `{"kind": "admin"|"leader"|"member", "name": "<agent>", "space"?: "<member's space>", "spaces": [{"name", "role": "admin"|"leader"|"member"}]}`. From 4a the admin has a name (`"admin"` or minted); from 4d its joined spaces are listed with role `admin`.
- `bus_list_spaces` → `[{"name", "leaders": [..], "admins": [..], "members": n, "joined": bool}]` [4d]; `members` counts role `member` only; `admins` = joined admins.
- `bus_list_participants` → `[{"name", "role"}]`, order admins, leaders, members, each by name [4d]; `+ "connected": bool` [12].

### 3.3 Invites [2, 7]

- `bus_invite_member` task error: `invalid "task must be 1..65536 bytes"` (was `body …`) [2]. Result unchanged `{name, space, code, expires_at}`.
- `bus_invite_member` description: task "delivered as a direct message from you when the code is accepted" [7].

### 3.4 Receipts and presence [12] — Fixed, PO-9 (`omp-parity/presence.feature`)

- `bus_send_direct` → `{"ok": true, "receipts": [{"to": "peer", "delivery": "live"|"queued"}]}`.
- `bus_send_topic` → same; one receipt per recipient except the sender, sorted by name; `[]` when none.
- `bus_add_post` → `{"postId": 7, "receipts": [...]}` (subscribers except the author).
- `live` = recipient holds its stream slot or is blocked in a server `bus_wait` right after commit; else `queued`. Delivery is exactly-once either way.
- `bus_list_participants` and overview participants: `"connected": bool`, per agent across spaces.

---

## 4. Inbox entry and stream

One `Entry` shape everywhere: SSE `event: inbox` data, `bus_inbox_read` element, `bus_wait` element, `GET /web/api/inbox` element, MCP inbox resource (`inbox/entry.go:15-30`). One field is added by slice D1: `class`.

```json
{"id": 12, "space": "demo", "type": "direct_message", "from": "lead", "critical": false,
 "class": "queue",
 "topic": "build", "blackboard": "plan", "threadId": 3, "threadTitle": "design",
 "body": "…", "createdAt": "2026-10-26T10:00:00.123Z"}
```
`topic`, `blackboard`, `threadId`, `threadTitle`, `body` are omitted when empty; `class` is **always present** (from D1). `type` ∈ `direct_message`, `topic_message`, `topic_closed`, `thread_post`, `thread_removed`, `blackboard_removed`. No new type.

### 4.1 Delivery class [D1] — Fixed, PO-5 (`mail/delivery-class.feature`)

Field name **`class`** (spec-verbatim, `delivery-class.feature:12,43`); it is unrelated to the receipt field `delivery` (`live`/`queued`, §3.4).

| `class` | Meaning (client-neutral, `delivery-class.feature:13-15`) | omp mapping (extension) |
|---|---|---|
| `interrupt` | Deliver into the running turn now; wake an idle agent. | `steer` |
| `queue` | Deliver at the next safe point of the running turn (between tool calls); wake an idle agent. | `aside` |
| `deferred` | Never interrupts and never wakes; becomes part of the agent's next turn. | `nextTurn` |

Server rule, decided in exactly one place (`server/internal/inbox/class.go`, `ClassOf(t Type, critical bool) Class`):

| Entry | Class |
|---|---|
| any entry with `critical: true` (today only `direct_message`, `topic_message` can be critical) | `interrupt` |
| `direct_message`, `topic_message`, `thread_post` | `queue` |
| `topic_closed`, `thread_removed`, `blackboard_removed` (lifecycle notices) | `deferred` |

- **Derived, not stored** [AD-15]: the class is a pure function of two stored columns (`type`, `critical`), so storing it would duplicate data that can drift; deriving needs no migration, and a policy change applies uniformly to the backlog and to every surface (SSE, `bus_inbox_read`, `bus_wait`, web) because all of them build entries through the same scan/insert path. Observably this is the spec's "assigns the class when it stores an entry" (`delivery-class.feature:39`): the mapping is fixed per release, and an entry's type and critical flag never change after insert. If a future entry ever needs a per-entry choice not expressible from stored columns, that slice adds a numbered migration and stores it.
- **Extensibility**: a new entry type is added to `classByType` in `class.go`; a test enumerates every `Type` constant and fails if one has no class. Producers never set the class.
- **Clients**: an unknown class value is treated as `queue` (`delivery-class.feature:94-97`). The field ships in the protocol-2 release, so the omp extension (minimum 2) never sees an entry without it. MCP/HTTP clients: informational; the guide tells agents to handle `interrupt` first.
- **Critical vs class**: `critical` stays in the entry (MCP and other clients read it, `tool-descriptions.feature:53`); `class` is what a client acts on.

### 4.2 Entry notes and stream

- **Invite task** [7]: a `direct_message` with `from` = inviter name (leader or admin, even if it left since), `critical: false` (so `class: "queue"`, `delivery-class.feature:57-60`), `body` = task, in the member's space; stored and published in the accept transaction.
- **`[via subagent <id>]`**: not a server concern. The extension prefixes the post/message body; the prefix counts toward the 65536-byte body limit (`omp-parity/omp-subagents.feature:134-155`); author = the token's agent; the author gets no notification of its own post (`blackboards/posts.go:28`).
- **Stream** `GET /v1/inbox/stream`: `text/event-stream`, `Cache-Control: no-store`, frames `id: <id>\nevent: inbox\ndata: <Entry>\n\n`, `: ping` every 20 s, one slot per agent → second stream `409 conflict "agent <name> already has an active inbox stream"` (`inbox/sse.go`, `inbox/handler.go:32-35`). From 4d admins may open it. A web session cookie alone → 401.
- **Order and replay (CR-8):** within one connection frames go out in strictly ascending `id`: first the whole unacked backlog of the caller's scope from the lowest id, then live entries; each entry at most once per connection (`inbox/stream.go:12-47`; pinned today by `TestStreamDelivery`, which D1 extends with a reconnect case). Every new connection replays from the lowest unacked id (`Last-Event-ID` is ignored). Ids are global, so a member's ids have gaps; never assume contiguity. The ascending order relies on the single SQLite writer committing ids in order; a change to that is a contract change.
- **Ack** `bus_inbox_ack {upTo}` → `{"acked": n}`; deletes every entry `id <= upTo` in the caller's scope (members: their space). Unchanged; the contiguous watermark is the extension's rule.
- **Scope**: leader and admin tokens see all their spaces; member tokens see one space (`inbox/service.go:116-122`).

---

## 5. `POST /v1/invites/accept` [7]

`Authorization: Bearer <code>`, no body. 200:
```json
{"name": "worker", "space": "demo", "token": "amb_…"}
```
`task` is gone (breaking for clients that read it; the task arrives as the member's first inbox entry, §4). Errors unchanged: `unauthenticated "invalid or expired invite code"`, `conflict "agent x is a leader agent"` (code kept, nothing delivered).

---

## 6. `GET /healthz` and protocol versioning [7]

- `GET /healthz`, no auth → `200`, `Content-Type: text/plain; charset=utf-8`, body `ok`, header **`Agent-Bus-Protocol: 2`**.
- The value is an integer. A response without the header = legacy server (the extension reports it as `none`). [AD-1]
- Source of truth: `server/internal/server/protocol.go` `const Protocol = 2`.
- Bump policy: at most +1 per release, by the first slice after a release tag that adds or changes a contract the extension depends on (a tool it calls, a result field it reads, an entry it renders, an endpoint). Additive server changes the extension ignores do not bump. `protocol.go` keeps one comment line per value.
- History: `2` = invite task as direct message, accept without `task`, kind/role `admin`, `bus_list_spaces.admins`, entry field `class` (the release containing 1–7 and D1). Next planned: `3` = the release containing 8/9/11 (`bus_space_overview`, server `bus_wait`, `GET /v1/guide`) if it is not the same release.
- The extension's minimum is its own constant (Arch-client). Old extension + new server needs no check (additive); the invite task then arrives via the old extension's inbox stream as a normal message.

---

## 7. Guide [11]

### 7.1 `GET /v1/guide`
- Auth: bearer (any kind, incl. admin), as specified in `agent-guide.feature:48-51` (guidance Q10 = Q-A5 in `QUESTIONS.md`, confirm-default: bearer unless the PO says otherwise). No bearer → `401 {"error":{"code":"unauthenticated","message":"bearer token required"}}`.
- 200, `Content-Type: text/markdown; charset=utf-8`, body = `server/internal/guide/guide.md` verbatim (no JSON wrapper).
- Older servers: `404` (no route). Guarded by the protocol check anyway.

### 7.2 MCP `instructions`
- `InitializeResult.instructions` (and 2026-07-28 discover) = the same guide text, identical for every token, ≤ 4000 characters (`agent-guide.feature:22-34`). Static; not per token.

### 7.3 TOOLS.md
- Section "Working on the bus" = guide text; golden test fails on drift.

---

## 8. Admin endpoints (loopback) [6a] — Fixed (`admin-tokens.feature:290-319`)

| Request | Result |
|---|---|
| non-loopback, any token | 403 `forbidden "admin endpoints are only available from localhost"` |
| loopback, leader/member token | 403 `forbidden "admin token required"` |
| loopback, minted admin, `POST /v1/admin/admin-token` | 403 `forbidden "only the bootstrap admin may replace the instance admin token"` |
| loopback, any admin, `POST /v1/admin/shutdown` | 200 `{"ok": true}` |
| loopback, bootstrap admin, `POST /v1/admin/admin-token {"token"}` | 200 `{"ok": true}`; bootstrap sessions/tokens switch, minted admins unaffected |

Admin tools have no loopback rule (`POST /v1/tools/bus_mint_admin` from `10.0.0.2` → 200).
MCP token user id: `admin:<agent id>` for every admin (was `admin:0`, `mcpserver/verifier.go:27`).

---

## 9. "You have mail" [13a, 13b] — Fixed, PO-7 incl. critical re-notify (`mail/mail-notice.feature`, `mail/mail-hint.feature`)

- RPC: `{"result": …, "mail": {"waiting": 2, "critical": 0, "spaces": ["alpha", "beta"]}}` on the first tool result after an unnoticed edge in the caller's scope; key **absent** otherwise. Never on `bus_inbox_ack`'s own removed entries.
- MCP: an extra trailing `TextContent`: `[agent-bus] You have mail: <n> waiting in <space>[, <space>…][ (<k> critical)]. Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}.`; `structuredContent` unchanged.
- `bus_inbox_read` / `bus_wait` returning the edge entry, or the SSE stream sending it, marks it noticed. The web GUI never marks and never carries `mail`. The omp extension strips `mail`.

---

## 10. Web GUI backend `/web` [W1a, W1b, W2, W3, 10] (`web/*.feature`, `web/analysis.md:300-414`)

Served only on the GUI's own listener `AMB_WEB_ADDR` (PO ruling replacing Q-A1): any `host:port`; empty or `off` disables the GUI; default `127.0.0.1:8081` [AD-16]. The operator controls exposure by choosing the address. The API listener (`AMB_ADDR`) serves no `/web` path (404), and the GUI listener serves only `/web/*` (everything else 404). Graceful shutdown stops both listeners. Errors use the §0 envelope and statuses. A non-GET method on a GET-only route → 405 (mux default).

### 10.1 Session [W1a]

| Request | Response |
|---|---|
| `POST /web/session`, header `Authorization: Bearer <token>`, no body | 200 `{"kind": "admin", "name": "ops", "space"?: "demo", "csrf": "<43 base64url>"}` + `Set-Cookie: amb_session=<opaque>; Path=/web; HttpOnly; SameSite=Strict; Max-Age=28800[; Secure]` (CR-11: `Path=/web`) |
| `GET /web/session` (cookie) (CR-G1) | 200 same body as sign-in (same `csrf` for the session's life); 401 `unauthenticated "session required"` without a valid session |
| unknown / empty / non-Bearer token | 401 `unauthenticated "bearer token required"` or `"invalid token"`; no cookie; token never echoed |
| > 5 failed sign-ins per IP per minute | 429 `{"error":{"code":"rate_limited","message":"too many failed sign-ins; try again later"}}` [AD-9] |
| `DELETE /web/session` (cookie) | 200 `{"ok": true}` + cookie cleared (`Max-Age=0`) |
| any `/web/api/*` without a valid session | 401 `unauthenticated "session required"` |
| session whose token stopped authenticating (rotate, revoke) | 401 `unauthenticated "invalid token"`; session dropped |

Leader and member tokens may sign in with their normal reach. Sessions are in memory (restart = sign in again), 8 h absolute [AD-8].

### 10.2 Reads [W1b]

`GET /web/api/state`:
```json
{"you": <bus_whoami result>,
 "spaces": [{"name": "demo", "leaders": ["lead"], "admins": [], "members": 1, "joined": false, "invites": 1}],
 "inbox": {"waiting": 3}}
```
`invites` (count of pending codes) present only for spaces where the caller has leader rights (leader of the space or admin).

`GET /web/api/state?space=demo`:
```json
{"you": <bus_whoami result>,
 "overview": <bus_space_overview result>,
 "invites": [{"id": "3f9a0c1d2e4b5a67", "name": "worker", "invitedBy": "lead", "expiresAt": "2026-…Z"}],
 "inbox": {"waiting": 3}}
```
No message bodies, never an invite code. `invites` only for leader-rights sessions (absent for members and non-leading participants); `id` from slice 10. Errors: unknown space 404; non-participant leader 403; no session 401.

| Read | Response |
|---|---|
| `GET /web/api/spaces/{space}/blackboards/{board}/threads` | `bus_list_threads` result |
| `GET /web/api/spaces/{space}/threads/{id}` | `bus_read_thread` result (404 for an id of another space) |
| `GET /web/api/inbox?after=&limit=` | `[Entry]` of the signed-in agent's own scope, exactly like `bus_inbox_read` (CR-G4: member sessions one space, leader/admin sessions all their spaces); `limit` 1..500, default 100; never acks, never marks noticed; mutating methods 405 |

### 10.3 Admin area [W2] (`web/gui-admins.feature`) — admin sessions only, else 403 `forbidden "admin token required"`

| Request | Response |
|---|---|
| `GET /web/api/admins` | `bus_list_admins` result |
| `POST /web/api/admins` `{"name"}` + `X-CSRF-Token` | `bus_mint_admin` result (token shown once) |
| `POST /web/api/admins/{name}/revoke` + `X-CSRF-Token` | `bus_revoke_admin` result |

### 10.4 Mutations [W3] (`web/gui-mutations.feature`) — PO-1 log line, PO-4 delete with confirmation

All need the session and `X-CSRF-Token` equal to the session's `csrf` (missing/foreign → 403 `forbidden "invalid CSRF token"`, no side effect).

| Request body | Tool | Result |
|---|---|---|
| `POST /web/api/spaces/{space}/invites` `{"name", "task"?}` | `bus_invite_member` | `{name, space, code, expires_at}` |
| `POST /web/api/spaces/{space}/revoke` `{"name", "confirm": "<space>/<name>"}` | `bus_revoke_member` | `{deleted}`; bad/missing confirm → 400 `invalid "confirm the revoke of <name> in space <space>"` |
| `POST /web/api/spaces/{space}/threads/{id}/posts` `{"body"}` | `bus_add_post` | `{postId[, receipts]}` |
| `POST /web/api/spaces/{space}/topics/{topic}/messages` `{"body", "critical"?}` | `bus_send_topic` | `{ok[, receipts]}` |
| `DELETE /web/api/spaces/{space}` `{"confirm": "<space>"}` | `bus_delete_space` | `{ok}`; bad/missing confirm → 400 `invalid "confirm the deletion of space <space>"` |

Each successful mutation logs `msg="web mutation" actor action space target` (PO-1: log line only, no table, no actions list).

### 10.5 Static assets [W1a] [AD-14]
- `server/internal/web/embed.go`: `//go:embed all:dist`. Tracked: `server/internal/web/dist/.gitkeep` and `server/internal/web/.gitignore` (`dist/*`, `!dist/.gitkeep`); slice W1a also changes the root `.gitignore` line `dist/` to `/dist/` so the `.gitkeep` can be added. The GUI build (Arch-client) writes `dist/index.html` + `dist/assets/*`; nothing generated is tracked (CR-10). This is PO-10's committed placeholder (approved, option b).
- `GET /web/` → `dist/index.html`; when it is missing (plain `go build` without bun) → a built-in fallback page (200, "The web GUI was not built; run the GUI build."; static HTML with no inline script or style, so it passes the CSP below). `GET /web/assets/*` → hashed bundles. Nothing else is served from the embed: `.gitkeep` and any other `/web/<path>` outside `/web/session` and `/web/api/*` → 404. Hash routing in the SPA: no server fallback route.
- Headers (CR-G3) on every `/web/*` response: `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`. `index.html` and the fallback: `Cache-Control: no-cache`; `assets/*`: `Cache-Control: public, max-age=31536000, immutable`; `/web/session` and `/web/api/*`: `Cache-Control: no-store`.
- Not provided (CR-G2 declined for now): `AMB_WEB_DIR` disk serving — it would be a second `platform/config` switch for a dev-only need; GUI development builds into `dist/` and restarts the server. Re-open if the GUI loop proves slow (Q-A7).

### 10.6 Pending invites: list and cancel [10] — PO-3 (`web/gui-invite-cancel.feature`, written by Spec-review)

No tool: neither MCP nor `/v1/tools` nor omp can cancel a single code; `bus_revoke_member` still cancels all pending codes of a name. The GUI is the only client.

| Request | Response |
|---|---|
| `GET /web/api/spaces/{space}/invites` | 200 `[{"id", "name", "expiresAt", "invitedBy"}]`, soonest expiry first; never the code; redeemed/expired/cancelled codes are gone |
| `DELETE /web/api/spaces/{space}/invites/{id}`, no body, `X-CSRF-Token` | 200 `{"cancelled": true}`; `{"cancelled": false}` for an unknown, malformed, redeemed, expired, already cancelled id or an id of another space (never reveals existence) |

Routes are spec-verbatim (`gui-invite-cancel.feature:17-19`).
- Invite id: opaque, 16 lowercase hex chars (`[0-9a-f]{16}`, 8 random bytes), generated when the invite is created (tool or GUI), stored in `invites.id` (migration `NNNN_invites_id`, number at merge, `schema.md` §2.3). Not derivable from the code; the code stays hashed. `bus_invite_member`'s result is unchanged (no `id`).
- Access (both routes): leader-of-space and admin sessions. Member session → 403 `forbidden "only leaders may list or cancel invites"`; leader not in the space → 403 `forbidden "not a participant of space demo"`; unknown space → 404 `not_found "space nope not found"`; no session 401; cancel without/with foreign CSRF token → 403, code still valid.
- Effects of a cancel: deletes exactly that `invites` row (`space_id` and `id` must both match); the code then fails at accept with `unauthenticated "invalid or expired invite code"`; other codes of the same name and joined members are untouched. One `web mutation` log line `actor=<session agent> action=cancel-invite space=<space> target=<invited name>`, only when something was cancelled.
- Service: `invites.Service.ListPending(ctx, p, space) ([]Pending, error)`; `invites.Service.Cancel(ctx, p, space, id string) (name string, cancelled bool, err error)` (the name feeds the log line).

---

## 11. Error message catalogue

New (all existing kinds):

| Kind | Message | Slice |
|---|---|---|
| invalid | `task must be 1..65536 bytes` | 2 |
| conflict | `agent ops is an admin agent` (article-aware kind conflict) | 4a |
| forbidden | `join space demo first; only participants receive messages` | 4c |
| conflict | `ops is an admin of space demo; admins cannot be revoked from a space` | 4d |
| conflict | `admin is the bootstrap admin; its token is the instance admin token` | 6a |
| forbidden | `only the bootstrap admin may replace the instance admin token` | 6a |
| forbidden | `admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token` | 6b |
| invalid | `timeoutSeconds must be 1..300` | 9 |
| forbidden | `only leaders may list or cancel invites` (web invite list/cancel) | 10 |
| unauthenticated | `session required` | W1a |
| forbidden | `invalid CSRF token` | W2 |
| invalid | `confirm the revoke of <name> in space <space>` / `confirm the deletion of space <space>` | W3 |
| (429) `rate_limited` | `too many failed sign-ins; try again later` | W1a |

Removed: `admin token cannot act in spaces; mint a leader with bus_mint_leader` (4c), `admin token has no inbox; mint a leader with bus_mint_leader` (4d). Narrowed: `last leader cannot leave; delete the space instead` only while other participants remain (5). Unchanged and still asserted: `only leaders may send to all`, `only leaders may send critical messages`, `space demo has other leaders; they must leave first` (leader caller), `not a participant of space demo`, `token is not valid for space demo`, `invalid or expired invite code`.

---

## 12. Contract requests ledger

Every contract request from Arch-client, checked against this file by the architecture lead (2026-10-26). "Section" is where the contract lives here; "Slice" names the server slice that implements or pins it, then the client slice(s) that consume it. `client.md` §12 mirrors this table.

| Id | Request | Decision | Reason | Section | Slice |
|---|---|---|---|---|---|
| CR-1 | `/healthz` protocol header, source, start value, bump policy | Accepted | needed for the too-old-server refusal (R2 of REVIEW) | §6 | 7; E9a |
| CR-2 | `bus_wait` result and one Entry shape | Accepted | spec-verbatim (`mail/bus-wait.feature`) | §2.5, §4 | 9, D1; E6 |
| CR-3 | whoami / participants / list_spaces shapes | Accepted | spec-verbatim admin shapes | §3.2 | 4a, 4d, 12; E8a, E8b |
| CR-4 | `GET /v1/guide` | Accepted | spec-verbatim (`agent-guide.feature:48-51`); bearer per Q-A5 | §7.1 | 11; E8b |
| CR-5 | receipts and `mail` | Accepted | PO-7, PO-9 | §3.4, §9 | 12, 13b; E10d, E7 |
| CR-6 | accept response without `task` | Accepted | the task arrives as the first DM (first-prompt rules) | §5 | 7; E9b |
| CR-7a | web session, reads, admin area, mutations, static assets | Accepted | PO-10 (b), web specs | §10.1-10.5 | W1a, W1b, W2, W3; GUI-1…GUI-6 |
| CR-7 | `bus_wait` survives a 300 s call (write/idle timeout > 300 s + margin) | Accepted, as "no server write/idle timeout at all" | stronger than requested and true today (`main.go:145`); slice 9 pins it with a test and documents ≥ 310 s for reverse proxies; clients use `timeoutSeconds + 10 s` | §2.5 Timeouts | 9; E6 |
| CR-8 | SSE frames in ascending id order, backlog first, replay from the lowest unacked id | Accepted | today's behaviour (`inbox/stream.go:12-47`, `TestStreamDelivery`); D1 adds the reconnect/`Last-Event-ID` case; 13b (which edits `stream.go`) must keep it green | §4.2 Order and replay | D1 (pin), 13b (keep); E2 |
| CR-9 | `GET /v1/tools` keeps listing `bus_wait` and `bus_inbox_ack` with schemas | Accepted | today's behaviour (`rpc/handler.go:34-37`, unfiltered); the extension withholds `bus_inbox_ack` itself (E7), the server never filters | §2.5 Registry | 9 (pin per token kind); E6, E7 |
| CR-10 | `//go:embed all:dist` + `dist/.gitkeep` + Go fallback page, no tracked `index.html` | Accepted ([AD-14]) | a tracked placeholder would be dirtied by every local GUI build; the tracked `.gitkeep` is PO-10's committed placeholder, and `go build` still works without bun; `all:` is required so the embed has a file; `.gitkeep` is never served | §10.5 | W1a (incl. root `.gitignore` `dist/` → `/dist/`); GUI-1 |
| CR-11 | session cookie `Path=/web` | Accepted | the cookie never reaches `/v1/*` (a cookie alone is never a bearer); the GUI calls only `/web/*` | §10.1 | W1a; GUI-2 |
| CR-G1 | `GET /web/session` → `{kind, name, space?, csrf}` | Accepted | restore after reload without storing the token or the CSRF value in the browser; same `csrf` for the session's life | §10.1 | W1a; GUI-2 |
| CR-G2 | dev-only `AMB_WEB_DIR` disk serving | Rejected (declined for now) | a second `platform/config` switch for a dev-only need; no spec needs it; rebuild + restart is enough (Q-A7) | §10.5 | — |
| CR-G3 | CSP and cache headers on `/web/*` | Accepted | defence in depth for a page that holds an admin session; immutable hashed assets; secrets (`/web/session`, minted tokens, invite codes) never cached; the fallback page is CSP-clean | §10.5 | W1a; GUI-1 (SPA without inline script/style) |
| CR-G4 | `GET /web/api/inbox` has the scope of `bus_inbox_read` | Accepted | spec (`web/gui-inbox.feature`); one scope rule for every surface; never acks, never marks noticed | §10.2 | W1b; GUI-4 |
| CR-12 | delivery class field on every entry | Accepted (PO-5) | field `class` (spec-verbatim), derived by `inbox.ClassOf` ([AD-15]); ships in the protocol-2 release | §4.1 | D1; E1 |
| CR-13 | invite list and cancel by id | Accepted (PO-3) | no tool; web endpoint with an opaque id; `state` lists pending invites from W1b, the `id` arrives with 10 | §10.2, §10.6 | W1b, 10; GUI-6 |

No contract request is open.
