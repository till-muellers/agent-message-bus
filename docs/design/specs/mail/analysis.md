# Mail delivery: "you have mail" and MCP push — analysis

BA output for `tmp/TODO.md` § "Mail delivery":
- item 1 "Notify 'you have mail' instead of making agents poll";
- item 3 "Let the MCP server push messages".

Item 2 (non-critical straight into the omp input queue) belongs to `docs/design/specs/omp-parity/`. Only its interactions are covered here (§4, §10).

**Round 2** applies the PO rulings of 2026-10-26:
1. The client bridge is parked (Appendix A).
2. omp delivery is `aside` for normal entries and `steer` for critical ones, with ack on consumption (§4).
3. There is one long-poll seam, `bus_wait` (§5.1).
4. The skill is retired; the guide is served as MCP `instructions` (§7).
5. **Superseded:** the round-3 "send-only `@admin` superuser" is replaced by the admin role of `docs/design/specs/admin/`: admins are named agents with their own inbox, reachable only in spaces they joined (§8, rewritten in round 4).

Files in this folder:

| File | Content |
|---|---|
| `analysis.md` | This document |
| `research-clients.md` | Full client research: one section per client, every claim with URL and quote. Done 2026-10-26 by a delegated research agent; the key Claude Code channel claims were re-checked against [channels-reference](https://code.claude.com/docs/en/channels-reference) |
| `mail-notice.feature` | Edge-triggered notice semantics (server, transport-agnostic), including admin-sent mail |
| `mail-hint.feature` | **New.** First notice channel: a one-time "you have mail" hint on the next bus tool result. Works in every client |
| `bus-wait.feature` | **New, replaces `inbox-wait.feature`.** The single long-poll tool `bus_wait`: server contract, with the omp local implementation behind the same contract |
| `omp-mail-delivery.feature` | **Rewritten.** How the notice and the cumulative ack interact with omp's `aside`/`steer` delivery and ack on consumption |
| `mcp-inbox-resource.feature` | MCP inbox resource: `resources/subscribe`, `notifications/resources/updated`, `subscriptions/listen` |
| `parked-client-bridge.feature` | **Parked / future** (was `client-bridge.feature`): local helper `agent-message-bus inbox wait|watch` for Claude Code hooks/monitors |
| `delivery-class.feature` | **New (PO-5, round 6).** Every inbox entry carries a server-assigned delivery class `interrupt` \| `queue` \| `deferred`; clients map classes to their mechanisms (omp: steer/aside/nextTurn) |

Nothing here was run against a real client. Claims I did not see in source or docs are marked [INFERENCE].

---

## 0. Summary

- **The PO is right.** In none of the ten external MCP clients researched does standard MCP push reach the model or wake an idle session (§2). The only client on this machine that does both is **omp's own MCP client**, and only with `mcp.notifications` turned on (default off).
- **The notice is a server concept.** It is defined once in the inbox: an *edge* is the delivery that made an empty mailbox scope non-empty. The edge is stored on the entry, together with whether the agent has been told or has looked since. Each channel decides how to show it.
- **omp shows full entries, never notices.** Normal entries arrive as `aside`, critical ones as `steer` (omp-parity). Acking on consumption keeps entries in the mailbox until they reach the context, so fewer entries are edges. omp consumes no edges, so nothing changes for omp. The ack must be a **contiguous watermark**: ack is cumulative, and a critical steer overtakes queued asides (§4).
- **One long-poll seam: `bus_wait`.** It is a server registry tool for every client, and the omp extension shadows it with a local implementation behind the same contract. `bus_inbox_read` stays non-blocking (§5.1).
- **Recommended sequence:**
  1. Edge detection (N1).
  2. `bus_wait`.
  3. The tool-result hint (M11), the first notice channel. It is universal and gives item 1 to every client during a turn.
  4. Later: the MCP inbox resource (M2). It is standards-track and works in omp's native client, but it needs stateful MCP sessions.
- **Parked:** the client bridge (Appendix A).
- **Rejected:** logging notifications, sampling, elicitation and the `tools/list_changed` trick.
- **Watch:** Claude Code channels and MCP Tasks.
- **Guide:** the mail section needs six lines (§7).
- **Admins (round 4, `docs/design/specs/admin/`):** admins are named agents of kind `admin` with their own mailbox. Their scope spans every space they joined, like a leader's. They get edges, hints, `bus_wait` and the inbox resource like any agent, and they send direct, `all` and critical messages under their own name (§8).

---

## 1. Current state (file:line)

### Server
| What | Where |
|---|---|
| Entries are inserted in the producer's transaction. After commit, **one empty wake-up signal per recipient agent** goes to broker topic `inbox/<agentID>` | `server/internal/inbox/deliver.go:12-27`, `service.go:41`, `service.go:59-70` |
| Read: id-ordered page; members see only their token's space | `service.go:74-83`, `service.go:116-122`, `repository.go:16-21` |
| **Ack is cumulative and destructive:** `DELETE … WHERE agent_id=? AND id<=? AND (space filter)`. There is **no read cursor**, and ack publishes nothing | `service.go:85-92`, `repository.go:24`, `repository.go:65-75` |
| Departure drops a member's entries of that space | `service.go:94-97`, `repository.go:26`, caller `spaces/service.go:212` |
| Today the admin has no inbox (`forbidden "admin token has no inbox; …"`), and `Principal{Kind: admin}` has no `AgentID` or `AgentName`. **Target (admin role, `docs/design/specs/admin/`):** admins are named agents with an inbox; this row is history | `service.go:99-104`, `auth/principal.go:12-24`, `migrations/0001_init.up.sql:1` |
| SSE stream: subscribe → drain the backlog → on each signal, coalesce and drain after `lastSent` ("exactly once, in id order"); ping every 20 s | `stream.go:12-28`, `:31-47`, `:50-71`, `:75-86`; `sse.go:30`; `service.go:18` |
| One stream slot per agent (409 for a second one); streams end on `streamRoot` | `handler.go:26-44`, `slots.go:5-32` |
| Broker: in-process; publish never blocks; a slow subscriber is closed; buffer 256 | `platform/broker/memory.go:6-34`, `server/build.go:23` |
| `/mcp` is **stateless + JSON responses, tools only**. The principal travels in `TokenInfo.Extra`; `UserID = "<kind>:<agentID>"` | `mcpserver/server.go:19-29`, `mcpserver/verifier.go:17-31`, `transports.feature:178-180` |
| Inbox tools `bus_inbox_read {after, limit}` and `bus_inbox_ack {upTo}` | `server/internal/tools/inbox.go:20-30` |
| No HTTP write timeout. Shutdown waits up to 10 s for in-flight requests | `server/main.go:145`, `platform/httpserver/serve.go:14,28` |
| Specified behaviour | `server/spec/inbox.feature:152-163`, `:165-267`, `:269-311` |

### Extension (omp), today
| What | Where |
|---|---|
| For each `inbox` frame: `deliver(entry)`, then `bus_inbox_ack {upTo: id}` **at once**. A failed ack is only reported. Backoff 1 s→30 s; 401/403 stops the loop | `extension/lib/inbox.ts:59-77`, `:91-109` |
| `critical` → `steer`, otherwise `followUp`; `triggerTurn` only in `tui`/`rpc` | `extension/lib/deliver.ts:17-19` |
| `pi.sendMessage({customType:"agent-bus", content, display:true})` | `extension/lib/deliver.ts:22-25` |
| `[agent-bus] <space> · <where> · from <x>[ · CRITICAL]\n<body>` | `extension/lib/format.ts:21-23` |
| Subagents get neither the inbox tools nor the stream | `extension/lib/connection.ts:56-57`, `extension/lib/tools.ts:17` |

**Target per PO ruling 2** (owned by omp-parity, `docs/design/specs/omp-parity/omp-delivery.feature`):
- normal entries go as `aside` (next step boundary; wakes an idle session in tui/rpc), critical ones as `steer`;
- one-shot modes start no turn while idle: the entry is queued for the next turn (`nextTurn`) and counts as consumed only once it is part of a model request, otherwise it stays unacked;
- an entry is acked after it reached the model's context.

---

## 2. Research: which clients surface push to the model

Levels: **a** = the client receives it, **b** = the model sees it, **c** = an idle session wakes up. Evidence for each external cell is in `research-clients.md` §2–§4. The omp row comes from my reading of the omp source installed in this repo (`node_modules/@oh-my-pi/pi-coding-agent`, omp 18.12.6).

| Client | `resources/subscribe` + `updated` | `list_changed` | logging | long-poll tool (timeout / does progress reset it) | sampling / elicitation | vendor push or hooks | **idle wake from outside** |
|---|---|---|---|---|---|---|---|
| **omp 18.12.6** (own MCP client) | **yes a+b+c** with `mcp.notifications=true` (default **false**): subscribes to every listed resource, injects a debounced `[MCP notification] N resource(s) updated … Use read(path="mcp://<uri>")`, starts a turn when idle | yes (a) | (a), extension event only | 30 s default (`OMP_MCP_TIMEOUT_MS`); progress does **not** extend it | neither declared | `pi.on("mcp_notification")` | **yes** (setting required) |
| Claude Code v2.1.310 | no (#7252 closed not-planned) | yes (a) | unknown | ≈28 h wall clock, not extended by progress; the 5 min idle window (HTTP) is reset by progress; auto-background after 2 min | no / yes (a) | channels (b; c disputed #44380), `asyncRewake` hooks (c), monitors, MCP Tasks (c, beta) | **yes, vendor features only** |
| Codex CLI | no, logged only (#16159) | tools behind a flag | log file | 60 s fixed | no / yes (a) | hooks (b), `Stop` block | no |
| Cursor | subscribe sent (a), no injection | tools: no | MCP Logs | IDE: reset by progress; CLI: 60 s | no / yes | `additional_context` (b), `stop.followup_message` | no |
| VS Code Copilot (local) | (a), for watched files only | yes (a) | output log | progress shown; no MCP-layer timeout found | yes / yes (a) | hooks (b), `agentStop` | no |
| Gemini CLI | no | yes (a) | none | 600 s, no reset | no / no | `additionalContext` (b), `AfterAgent` | no |
| Claude Desktop / claude.ai | no (unsupported) | unknown | unknown | 240 s | no / unknown | none | no |
| Goose, Zed, Windsurf, Cline | no / unknown | tools (a) | Goose: UI | Goose 300 s, Cline 60 s | partial | Cline: context only | no |

omp evidence (under `node_modules/@oh-my-pi/pi-coding-agent/src/`):
- Setting: `mcp/settings.ts:46-69`.
- Auto-subscribe: `mcp/manager.ts:1875-1895`. Handling of `updated`: `mcp/manager.ts:1104-1113`.
- Injected text: `sdk.ts:469-490`. Enqueue: `sdk.ts:5504-5520`.
- Idle flush calls `agent.prompt`: `session/agent-session.ts:2132-2149`.
- GET stream; a 405 gives up: `mcp/client.ts:182-183`, `mcp/transports/http.ts:209-264`.
- Protocol 2025-11-25: `mcp/types.ts:181`. Timeout: `mcp/timeout.ts:3`. Progress: `mcp/types.ts:303-304`.
- Capabilities `roots` only: `mcp/client.ts:101-103`.
- `mcp_notification` event: `extensibility/extensions/types.ts:1201-1220`, `sdk.ts:5538-5540`.

### Verdict on the PO note
"Submitting events into a running session is not that easy via a pure MCP server": **correct.**
- Push is optional in every spec revision.
- The 2026-07-28 revision removes the GET stream and `resources/subscribe`.
- No external client puts push content into the model or wakes on it.
- **Exceptions:**
  1. A tool result reaches the model in every client, during a turn (`bus_wait`, the hint).
  2. omp's own client does a+b+c on `resources/updated`.
  3. Claude Code wakes only through non-standard features (channels, `asyncRewake` hooks, monitors, MCP Tasks). See `research-clients.md` §3.

### go-sdk v1.8.0 (`~/go/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/`)
| Fact | Where |
|---|---|
| Stateless: no session ID, a temporary session per request, GET/DELETE → 405, server→client requests rejected | `streamable.go:128-146`, `:373-386`; `streamable_server.go:113-122` |
| `JSONResponse` routes messages sent during a request to the standalone stream. Stateless mode has none, so **progress during a tool call is lost** with today's options | `streamable_server.go:132-134`, `streamable.go:1854-1856` |
| `subscriptions/listen` is always answered as SSE | `streamable.go:1678-1683` |
| 2026-07-28 is only negotiated on stateless transports | `streamable.go:868-876`, `shared.go:50-61` |
| `SubscribeHandler`/`UnsubscribeHandler` (as a pair); `resources.subscribe` is inferred | `server.go:108-111`, `:226-231`, `:687-695` |
| `ResourceUpdated(uri)` notifies **all** sessions subscribed to that URI; there is no per-session notify | `server.go:1180-1232`, `:1256-1321` |
| `NotifyProgress`; `Log` and sampling are deprecated (SEP-2577); server-initiated requests are forbidden at ≥ 2026-07-28 | `server.go:1520`, `:1813-1817`, `:1659-1664`, `:1613-1628` |
| Stateful sessions are bound to `TokenInfo.UserID` and reject calls made before `initialize` | `streamable.go:567-571`, `:709-719`; `server.go:1989-1991` |
| `SessionTimeout`, `EventStore`, `KeepAlive` | `streamable.go:160-172`; `server.go:100-107` |
| No MCP Tasks; no custom notification methods | `protocol.go` (no `tasks/*`); `research-clients.md` §3.1 |

### pi / omp extension API
- **`pi.events`** (`utils/event-bus.ts:3-33`, `types.ts:1963-1964`, one per process `main.ts:554, 2377`) is in-process fan-out between extensions. It doesn't reach the model, doesn't start turns and doesn't cross processes. **It doesn't help mail delivery.** omp-parity uses it for widgets (their S8).
- What matters is `pi.sendMessage(..., {deliverAs: "steer"|"followUp"|"nextTurn"|"aside"})` (`types.ts:1846-1864`), which is omp-parity's lever, and `pi.on("mcp_notification")`.

---

## 3. Item 1: "You have mail" (edge-triggered)

### 3.1 Semantics (`mail-notice.feature`)
- **Scope:** a leader or admin token covers all of that agent's entries; a member token covers the member's entries in that space. An admin's entries only ever come from spaces it joined.
- **Empty:** no unacknowledged entry in the scope. "Emptied" means acked, or dropped on departure.
- **Edge:** the delivery transaction that stores the first entry into an empty scope.
  - Entries into a non-empty scope are not edges.
  - A partial ack or a read doesn't re-arm.
- **Told/looked:** an edge entry stays "unnoticed" until a channel told the agent, or `bus_inbox_read`/`bus_wait` returned it (`mail-hint.feature`).
- **Level at subscribe:** a channel that starts listening on a non-empty scope gets one notice.
- **Content:** count, scopes, critical count, and "call `bus_inbox_read`, then `bus_inbox_ack`". No bodies.
- **Mail sent by an admin** follows the same rules (§8).

### 3.2 Where the edge is detected (rated 1–5: clean / simple / powerful)
| # | Seam | C | S | P | Justification |
|---|---|---|---|---|---|
| **N1** | **Edge (and "noticed") stored on the entry at insert**, inside `inbox.Deliver`'s transaction | **5** | 4 | **5** | Exact, because SQLite serialises writers. Survives restarts. Works with the stateless `/mcp`, since no session has to remember anything. Reuses the stream's id-ordered drain for dedupe. Cost: one numbered migration owned by `inbox` (two columns; numbered after `0001_schema`, never folded into it: `docs/design/specs/REVIEW.md` §11) and two `EXISTS` queries per recipient per delivery |
| N2 | Edge in the broker payload | 3 | 4 | 4 | No migration, but the empty wake-up signal (`inbox.feature:157`) becomes a protocol. Has no "noticed" state, so the hint (M11) would need N4 on top |
| N3 | Channel-side latch on recount | 3 | 4 | 2 | Misses re-arms when an ack and an insert coalesce (`stream.go:75-86`). Rejected |
| N4 | In-memory "notified" bit in `inbox.Service` | 2 | 3 | 3 | Hidden shared state; re-notifies after a restart |
| N5 | Extension-only | 2 | 3 | 1 | omp only, and omp doesn't need it |

**Recommendation: N1**, through the documented seam `inbox.Deliver` (`deliver.go:10-27`). It ships **together with its first consumer** (M11). Without a consumer it would be dead weight: omp doesn't use edges, and `bus_wait` returns entries.

---

## 4. Fit with omp delivery (PO ruling 2)

**omp target** (omp-parity):
- normal entries go as `aside`, critical ones as `steer`;
- one-shot modes start no turn while idle; the entry waits as `nextTurn` and stays unacked until it is part of a model request;
- **ack after the entry reached the model's context** (their S10/T7).

**Notice in omp: none.**
- omp shows every entry in full (`omp-mail-delivery.feature`, rule 1).
- A notice would repeat the entry, and the tool-result hint is dropped by the extension.
- The earlier option "notice mode for omp" (O2) is withdrawn: the ruling settles omp delivery as full push.

**What ack on consumption means for the edge:**
- The edge stays a fact about stored rows at insert time, so its definition doesn't change.
- An entry stays in the mailbox from the moment the stream hands it to the extension until it reaches the context (and, in one-shot idle mode, until a later session replays it).
- Entries stored meanwhile are **not** edges. So under omp, edges become rarer: one per "mailbox was fully consumed" period instead of almost one per entry with today's immediate ack.
- No omp channel consumes edges, so omp sees no difference.
- A non-omp channel on the same identity would see correct edges, but one identity belongs to one session (`extension/README.md:123-125`), so this doesn't happen in practice.
- **The hint would never fire for omp agents** in practice. The extension's own `bus_inbox_ack` calls don't carry hints (`mail-hint.feature`). Its other tool calls may get a `mail` field, which it drops.

**What it means for the cumulative ack (new finding, agreed with omp-parity):**
- `bus_inbox_ack {upTo}` deletes every entry with id ≤ `upTo` (`repository.go:24`).
- A critical `steer` reaches the context **before** asides queued earlier (`omp-parity/omp-delivery.feature:57`).
- Acking the critical entry's id would delete those asides. If the session then ends before they are injected, they are lost: the very loss S10 is meant to prevent.
- **Rule:** the extension acks a **contiguous watermark**, the highest id at which every entry it received with a lower or equal id has reached the context (`omp-mail-delivery.feature`, rule 2).
- **Accepted cost:** an entry that reached the context but sits above an unconsumed one is replayed after a crash. A duplicate beats a loss.
- **Not proposed:** per-entry ack on the server (`bus_inbox_ack {ids}`). It would remove the watermark, but it changes a shared contract that MCP clients rely on. Possible later.

| Option | C | S | P | Note |
|---|---|---|---|---|
| **O1 Full push (`aside`/`steer`) + watermark ack, no notice in omp** | 5 | 4 | 5 | The PO ruling plus the watermark. Delivery is owned by omp-parity, the watermark is specified here |
| O2 Notice mode for omp | — | — | — | Withdrawn by ruling 2 |
| O3 Per-entry ack on the server | 4 | 3 | 4 | Removes the watermark logic, but changes the ack contract for every client. Later, if ever |

---

## 5. Item 3: MCP push, and how the agent learns about mail

### 5.1 Decision: one long-poll seam, `bus_wait` (PO ruling 3)

Two proposals existed:
- my `bus_inbox_read {waitSeconds}`, server-side, for MCP clients;
- omp-parity's `bus_wait`, an extension-local tool with `from`/`threadId` filters that takes entries from the stream dispatcher.

**Decision: merge into one tool contract named `bus_wait`.** It is defined in the server registry and implemented twice behind that contract. omp-parity agreed (`agent://BA-omp-parity`, 2026-10-26).

**Contract:** `bus_wait {timeoutSeconds 1..300 (default 30), after?, from?, threadId?}` → `[entries]` in id order (at most 100); `[]` on timeout. Specified in `bus-wait.feature`.

| Side | Implementation |
|---|---|
| **Server** (`tools`, for MCP and `/v1/tools`) | Subscribe to `inbox/<agentID>`, read matching unacked entries after `after`, wait otherwise. **Never acks.** Non-matching entries stay in the mailbox. Ends on disconnect and on `streamRoot` |
| **omp extension** (local, omp-parity S5) | Same name, schema and result. The remote `bus_wait` is not registered (`registerRemoteTools` exclude, like `bus_spawn_worker`). Takes the entry from the stream dispatcher instead of injecting it, counts it as consumed and acks it through the watermark. Main session only |

**Why `bus_wait`, and not `bus_inbox_read {waitSeconds}`:**
1. **One name everywhere.** omp agents and MCP agents learn the same verb. The guide (§7) needs one line instead of two client-specific ones.
2. **It matches the idiom.** omp's own `wait` blocks for owed replies; "wait" is the verb agents already know (`omp-parity/analysis.md` G7).
3. **The reply case needs filters.** `from` and `threadId` serve "wait for my leader's answer" or "wait for the next post in thread 7". Bolting filters onto a pager muddles `bus_inbox_read`.
4. **`bus_inbox_read` stays a pure, non-blocking pager.** No argument changes its blocking behaviour; tool descriptions stay simple.
5. **omp can't use a server-side wait anyway.** The extension owns the stream and acks, so a server-side wait would race the stream and deliver entries twice. A contract implemented twice is the only design that works for both.

**Cost:** one more tool in the registry and in `TOOLS.md`. The final registry after every proposal is 34 server tools; `docs/design/specs/REVIEW.md` §11 gives the `transports.feature` count at each merge step.

**Limits:**
- Only during a turn: the model must call it.
- Client timeouts apply: Codex and the Cursor CLI 60 s, Desktop 240 s; the default of 30 s fits all.
- Progress during the wait needs SSE responses (M4).

### 5.2 Seams (rated 1–5)
| # | Seam | C | S | P | Reaches | Status |
|---|---|---|---|---|---|---|
| **M1** | **`bus_wait`** (§5.1) | 5 | 4 | 3 | b in every client, during a turn | **Recommended (2)** |
| **M11** | **Tool-result hint** (`mail-hint.feature`): the next bus tool result after an edge gets one line, "You have mail: n waiting in … Call `bus_inbox_read` …". For MCP it is an extra text block; for `/v1/tools` an additive `mail` field | 4 | 4 | 3 | b in every client, the next time the agent calls any bus tool | **Recommended (3)**. Upgraded from "optional": with N1's stored "noticed" state it needs no session state, and it is the only notice channel every client surfaces. It delivers item 1 to non-omp agents. Cost: a decorator in the transports layer (`internal/rpc`, `internal/mcpserver`) and one query per tool call |
| M2 | **Inbox resource + subscribe** (`mcp-inbox-resource.feature`) | 4 | 2 | 2 now / 4 later | a+b+c in omp native (setting on); a in VS Code and Cursor; nothing elsewhere | **Later (4)**. Standards-track (`subscriptions/listen` in 2026-07-28), but needs stateful sessions for ≤ 2025-11-25, a router, URIs per scope, and level-at-subscribe after SDK registration |
| M4 | Progress during `bus_wait` | 4 | 3 | 1 alone | keeps calls alive in Claude Code (idle window) and Cursor IDE | With M1, once tool calls answer as SSE (today's `JSONResponse` drops it, `streamable.go:1854`) |
| M12 | Guide lines in MCP `instructions` (§7) | 5 | 5 | 2 | b | Needed with M1/M11; carried by the guidance BA |
| M8 | Claude Code channel shim | 3 | 2 | 3 | b; c disputed (#44380) | Watch (research preview, stdio, dev flag, protocol pinning) |
| M9 | MCP Tasks for `bus_wait` | 4 | 1 | 3 | c in Claude Code CLI | Watch (go-sdk lacks tasks, go-sdk#626) |
| M3 | Logging notifications | 2 | 3 | 1 | log panels | Rejected (deprecated, SEP-2577) |
| M5 | Sampling | 1 | 2 | 0 | — | Rejected |
| M6 | Elicitation | 1 | 2 | 1 | the human | Rejected |
| M10 | `tools/list_changed` trick | 1 | 2 | 2 | next turn | Rejected |
| M7 | Client bridge | — | — | — | — | **Parked**, Appendix A |

### 5.3 Recommended sequence
1. **N1** edge + "noticed" columns (`inbox`), with `mail-notice.feature`, in a numbered migration after `0001_schema` (the next free number when the slice merges; `docs/design/specs/REVIEW.md` §11). Lands in the same release as step 3.
2. **`bus_wait`**, server tool (`bus-wait.feature`). The omp local shadow follows in omp-parity's phase 2.
3. **M11** tool-result hint (`mail-hint.feature`).
4. **M2** MCP inbox resource, when standards-track push is wanted (Q3).
5. **Watch:** M8, M9.

---

## 6. Requirement trades (explicit)

| # | As written | Proposed instead | Enables | Cost |
|---|---|---|---|---|
| T1 | "Push messages to MCP clients" | Push a **notice**; the agent pulls with `bus_inbox_read`, then `bus_inbox_ack` | The standard MCP seam (`resources/updated` carries only a URI). The mailbox stays the source of truth | One tool round trip per edge |
| T3 | "Don't poll" | **One blocking call** (`bus_wait`) when idle in a turn, plus a **hint** when active | Works in every client | Still calls; nothing wakes an idle non-omp session |
| T4 | "Notify again only after emptied" | The same, **plus: a critical entry always notifies** | Urgent orders don't hide behind a backlog | Can repeat while unacked (Q2) |
| T5 | "Emptied" (undefined) | **Empty = no unacked entries**; reading doesn't re-arm | No read cursor needed | Read but never acked means silence (mitigated by T4 and the guide) |
| T6 | Notice in omp too | omp shows full entries (`aside`/`steer`), no notices | No duplicates | — (settled by ruling 2) |
| T7 | "Notify" = push into the conversation | For non-omp clients: a **hint on the next tool result** | Universal, needs no client support | Arrives only when the agent calls a bus tool |
| T8 | Two wait tools (`waitSeconds`, `bus_wait`) | **One contract `bus_wait`**, two implementations | One verb for agents and the guide | One more tool |
| T2 | "The MCP server pushes" | Client-side helper | Idle wake in Claude Code | **Parked** (Appendix A) |

---

## 7. Mail lines for the server-built guide (PO ruling 4)

The guide is static for every token (`docs/design/specs/guidance/analysis.md` Q9) and is served as MCP `instructions`, as `GET /v1/guide` and in `TOOLS.md`. Its 4000-character budget is set by omp's cut-off (guidance E12). It must therefore hold both the omp and the non-omp line.

**Adopted (round 4):** the guidance BA merged these lines, shortened, into `docs/design/specs/guidance/guide-draft.md` ("## Mail"). That draft is now the source of the wording. The content the guide must keep from this BA:
- omp: entries arrive by themselves at the next step; only `CRITICAL` ones interrupt; they are acknowledged once seen; don't call `bus_inbox_read`.
- Other clients: act on the `You have mail` hint, read with `bus_inbox_read`, acknowledge with `bus_inbox_ack {upTo}` only what you handled.
- Nothing to do until mail comes: call `bus_wait {timeoutSeconds}` (`from`/`threadId` filters); 30 s is safe for every client.
- `CRITICAL` entries come from a leader or an admin.

**Conditional:**
- Only if M2 lands, add: "If your client injects a `bus://_inbox/…` resource update, it means the same as the hint."
- Replies to an admin that has not joined the space fail with `not_found`; the guide's "Members" section already tells members to report to a leader then.

**Tool descriptions** (moved into guidance's `tool-descriptions.feature` as rows):
- `bus_wait`: "Blocks until mail for you arrives or `timeoutSeconds` (1..300, default 30) pass; returns the entries (`[]` on timeout). Filter with `from`/`threadId`. Does not acknowledge: call `bus_inbox_ack` after handling (omp acknowledges for you)."
- `bus_inbox_read`: append "Never blocks; to wait for mail use `bus_wait`. Reading does not acknowledge."
- `bus_inbox_ack`: append "Deletes every entry with id ≤ upTo: acknowledge only what you have handled. Acknowledging everything lets the next mail notify you again."
- Access line of the inbox tools and `bus_wait`: "Leaders, members and admins" (`docs/design/specs/admin/analysis.md` §5).

**Not in the guide:** the client research, the hint field format and resource URIs. Tool results and the resource description carry those.

---

## 8. Admins (round 4; supersedes the round-3 "send-only `@admin`" design)

Today the admin can act in no space and has no inbox (`inbox/service.go:99-104`). It is not an agent: no row in `agents`, `AgentID` 0, no name (`auth/principal.go:19-24`).

**PO ruling (round 4), specified in `docs/design/specs/admin/`:** `admin` is a full agent kind. Every admin is a named agent (the bootstrap admin `admin`, more via `bus_mint_admin`) with its own inbox and stream slot. It acts as a leader in every space without joining, and it is reachable only in spaces it joined (participant role `admin`). The `@admin` reserved row is gone.

What that means for mail:
- **An admin has a mailbox** whose scope spans every space it joined, like a leader's (`mail-notice.feature`, admin rule). Edges, the hint (`mail-hint.feature`), `bus_wait` (`bus-wait.feature`), the stream and the MCP inbox resource (`mcp-inbox-resource.feature`, URI `bus://_inbox/<admin>`) work for it as for any agent.
- **An admin only receives where it joined.** It is not a recipient of `all` and cannot subscribe in spaces it has not joined (`docs/design/specs/admin/admin-messaging.feature`). So its scope never holds entries of other spaces, and departure cleanup keeps working.
- **An admin sends under its name** (direct, `all`, critical, in every space). For recipients these are ordinary entries: edges per recipient scope, the critical rule (T4, if the PO confirms Q2), and `steer` in omp.
- **Replies to an unjoined admin fail** with `not_found "participant ops not in space demo"`; the guide tells members to report to a leader then.
- **The web GUI reads an admin's inbox without marking edges** (`docs/design/specs/web/gui-inbox.feature`, `mail-hint.feature` "web GUI" rule), so the operator's agent session still gets its hint.
- **Effect on this design:** none beyond the scope definition. Edge detection (N1), the hint and `bus_wait` work per recipient scope and never look at the sender.

---

## 9. Open PO questions

1. ~~**Q2**~~ *Decided (PO-7):* a critical entry always notifies.
2. **Q3** M2 (stateful MCP + resource subscription): later (recommended) or now?
3. **Q5** `bus_wait`: are the bounds 1..300 and the default 30 right?
4. **Q7** Confirm "empty" = no unacked entries (T5).
5. ~~**Q8**~~ *Decided (PO-7):* persisted edge and "noticed" columns (N1), numbered migration after `0001_schema`.
6. ~~**Q13**~~ *Decided (PO-7):* the hint (M11) as an additive `mail` field on `/v1/tools` and an extra MCP text block.

Superseded in round 4 (admin role, `docs/design/specs/admin/`; see §8):
- **Q9–Q12** (send-only admin, `@admin` sender row, admin critical/`all`, no admin subscriptions). Admins now have an inbox, send under their own name, may send critical and to `all`, and subscribe only where they joined.

Withdrawn after round 2: Q1 (omp notice mode, settled by ruling 2), Q4 and Q6 (bridge, parked).

---

## 10. Risks

- **Cumulative ack + out-of-order consumption** (§4): without the watermark, omp loses entries. A duplicate after a crash is the accepted cost.
- **Read but never acked** → silence (T5). Mitigated by the guide line, T4 and `bus_wait` returning unacked entries at once.
- **Hint on every result** (M11): one extra query per tool call. The hint itself costs tokens only once per edge. RPC clients that validate result shapes strictly may choke on the additive field [INFERENCE].
- **`bus_wait` twins:** the server and extension implementations must keep the same schema. The extension should copy the schema from `GET /v1/tools` rather than redefine it, otherwise they drift [INFERENCE].
- **`bus_wait` lifecycle:** must stop on disconnect and `streamRoot` (shutdown waits 10 s, `serve.go:14`). Holds one broker subscription per waiting call.
- **Stateful MCP (M2):**
  - `SessionTimeout` and `KeepAlive` are required.
  - Close sessions on revocation via `spaces.RevocationHook` (`server/build.go:30`).
  - A member's space tokens share a `UserID` (`verifier.go:27`).
  - Calls made before `initialize` are rejected (router needed).
- **omp native MCP injection** is off by default and uses generic text, so the resource description must say "then `bus_inbox_ack`".
- **Headless modes never wake:** omp `-p` (entries stay unacked) and Claude Code `-p`.
- **Volatile ground:** Claude Code features are previews or betas; the 2026-07-28 spec drops legacy subscriptions. Nothing has been verified on a real client.

---

## 11. Proposed slicing (each a separate PR, behaviour first in `server/spec/`)

1. `tools`: `bus_wait` server tool + `bus-wait.feature` → `server/spec/`; regenerate `TOOLS.md`; update the `transports.feature` count (`docs/design/specs/REVIEW.md` §11, slice 9).
2. `inbox`: N1 migration (numbered after `0001_schema`, next free number at merge; never folded into 0001) and edge detection, and `rpc`/`mcpserver`: the M11 hint decorator. `mail-notice.feature` + `mail-hint.feature`. One release (`docs/design/specs/REVIEW.md` §11, slice 13).
3. `extension` (omp-parity owns it): `aside`/`steer`, watermark ack (`omp-mail-delivery.feature`), local `bus_wait`, drop the `mail` field.
4. Guide and tool descriptions (guidance BA): the §7 lines.
5. Later: `mcpserver` router + inbox resource (`mcp-inbox-resource.feature`); `transports.feature` "tools only" changes.

---

## 12. Interactions with parallel BAs

- **omp-parity:**
  - **Agreed:** one `bus_wait` contract (§5.1), and the watermark ack, adopted into their `omp-delivery.feature`.
  - **Theirs:** `aside`/`steer`, wake rules, ack on consumption, and the local `bus_wait` (main session only; subagents get blackboard tools only).
  - **Mine:** edge semantics, the hint, the server `bus_wait`.
  - Coordinate edits to `deliver.ts`/`inbox.ts`.
- **guidance:** the §7 lines replace `guide-draft.md:10-11`. Their tool-description fixes take the three description lines. Their D6 ("inbox tools mislead omp agents") is resolved by the omp guide line plus the descriptions.
  - On their open overlap (`guidance/analysis.md:202`): the worker's first task wakes it unchanged in omp, because it arrives as an `aside` entry. MCP workers see it on their next bus tool call (hint) or through `bus_wait`.
- **findings:** no overlap.
- **web GUI:** could show per-agent unread counts from the same scope query. Its inbox page reads through a service listing that does not mark edges as noticed, and its proxy of `/v1/tools` drops the `mail` field (`docs/design/specs/web/gui-inbox.feature`).
- **admin (`docs/design/specs/admin/`):** owns the admin role that §8 relies on.

---

## Appendix A — Parked / future (PO ruling 1: not implemented now)

**Client-side bridge `agent-message-bus inbox wait|watch` (seam M7, trade T2), spec `parked-client-bridge.feature`.**

What it would be:
- Two client subcommands of the bus binary.
- `inbox wait` long-polls with `bus_wait`, prints entries in the `[agent-bus]` format to stderr, acks them and exits 2. On timeout it exits 0.
- `inbox watch` holds the SSE stream, prints each entry and acks it.

Why it was interesting:
- It is the only documented, reliable idle wake for Claude Code: a `Stop`/`SessionStart` command hook with `asyncRewake: true` (exit 2 wakes the session; stderr is shown as a system reminder), or a plugin monitor whose lines reach Claude as notifications (`research-clients.md` §3.2-§3.3).
- Other clients would get context injection or a forced continuation at turn end through their own hooks.
- Rating: clean 4, simple 3, powerful 4 (Claude Code) / 3 (others).

Open points to settle if it comes back:
- Ack after printing vs. notice only (former Q6).
- Single-flight lock when `SessionStart` and `Stop` hooks overlap.
- Mail lost if Claude Code crashes after the ack.
- Whether this repo ships a Claude Code plugin with the hook and monitor config (former Q4).

It depends on `bus_wait`, which is recommended anyway, so parking it costs nothing later.
