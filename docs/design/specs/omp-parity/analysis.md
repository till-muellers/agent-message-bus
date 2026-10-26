# omp parity: bus messaging "almost exactly like" omp's own, plus blackboards

PO ask: *"For omp, I'd like the plugin to behave almost exactly like the regular message tools PLUS the cool blackboard."*
TODO (`tmp/TODO.md:36`): *"Later: send non-critical messages straight into the input queue (omp consumers) instead of keeping them in the mailbox, the same way omp passes messages between an agent and its subagents (the 'send' vs 'steer' distinction). Critical messages keep steering."*

Sources. omp source: `node_modules/@oh-my-pi/pi-coding-agent` **18.12.6** in this repo (the installed `omp` reports `omp/18.12.6`; `~/.bun/bin/omp.bunx` runs `@oh-my-pi/pi-coding-agent/dist/cli.js`). Paths below that start with `session/`, `irc/`, `internal-urls/`, `tools/`, `registry/`, `extensibility/`, `modes/`, `prompts/`, `task/`, `sdk.ts`, `main.ts` are relative to `node_modules/@oh-my-pi/pi-coding-agent/src/`. `pi-tui/…` and `pi-agent-core/…` are relative to `node_modules/@oh-my-pi/`. Upstream pi is `badlogic/pi-mono` (`packages/coding-agent`, `main`, fetched 2026-10-26). Claims I did not observe in code are marked **[INFERENCE]**.

**Round 2 PO rulings applied** (details in §6): (1) only `critical` steers; every normal message is an `aside`. (2) Subagents get only `bus_list_blackboards`, `bus_list_threads`, `bus_read_thread`, `bus_create_thread`, `bus_add_post`; everything else stays with the main interactive session; subagents talk to it via `write agent://<id>` / `wait` (§2.6, `omp-subagents.feature`). (3) Essential tools for the main session: §6.2. (4) The skill is retired (guidance BA owns the replacement); no skill update items here. (5) One long-poll seam: `bus_wait`, contract owned by the mail BA (§6.3).

Feature files in this folder:

| File | Covers | Where it lands |
|---|---|---|
| `omp-delivery.feature` | aside/steer delivery, wake, plan/print modes, ack timing | extension |
| `omp-rendering.feature` | message details, transcript card, envelope, reply hints, forgery | extension |
| `omp-tools.feature` | main-session tool presentation (loadMode, approval), `bus_wait` (shared contract), receipt texts, outgoing card | extension (+ server metadata) |
| `omp-subagents.feature` | subagents: blackboard read/post tools only, root identity, attribution line | extension |
| `presence.feature` | send receipts `live`/`queued`, `connected` in participant lists | server |
| `omp-blackboards.feature` | compact/coalesced post notices, non-waking lifecycle notices, status line, thread rendering | extension |
| `omp-events.feature` | publishing bus activity on `pi.events`, two-copy handshake | extension |
| `omp-bus-url.feature` | **option, not recommended now**: `bus://` in omp's read/write | extension (omp internals) |

---

## 1. Today's behaviour, mapped to code

| Behaviour | Code |
|---|---|
| Inbox stream held per session, reconnect with backoff 1 s → 30 s, stop on 401/403 | `extension/lib/inbox.ts:34-35, 49-51, 91-109` |
| Each SSE `inbox` frame is delivered, **then acked at once** (`bus_inbox_ack {upTo: id}`); ack failures are only reported | `extension/lib/inbox.ts:59-77` |
| Delivery policy: `critical` → `steer`, else `followUp`; `triggerTurn` only in `tui`/`rpc` | `extension/lib/deliver.ts:11-19` |
| Message shape: `customType: "agent-bus"`, `content` = formatted text, `display: true`, **no `details`** | `extension/lib/deliver.ts:22-26` |
| Text: `[agent-bus] <space> · <where> · from <from>[ · CRITICAL]\n<body>`; body not escaped, no reply hint | `extension/lib/format.ts:4-23` |
| No message renderer registered → omp's generic custom-message component | `extension/` (no `registerMessageRenderer`); omp fallback `modes/utils/ui-helpers.ts:370-374` |
| Server tools mirrored 1:1 as omp tools; no `loadMode`, `approval`, `renderCall`, `renderResult` | `extension/lib/tools.ts:30-43` |
| Subagents: no inbox tools and no stream, but **every other bus tool**, and `bus_spawn_worker` is activated too (`localTools` is switched on regardless of `view.sub`) | `extension/lib/tools.ts:16-17`, `extension/lib/connection.ts:48, 56-58` |
| Subagent credentials: own extension instance resolves its own; sees the accepted invite (keyed by the root session) and the stored token, but not env credentials (deleted at the parent's load) nor a `/bus promote` token (parent instance memory) | `extension/lib/credentials.ts:8-15, 76-83`, `extension/lib/session-store.ts:6, 46-48`, `extension/lib/env.ts:19-22` |
| Spawn `task` arrives as a plain user message | `extension/index.ts:101-105` |
| Server: stream sends backlog then live; unacked entries replay on reconnect; one stream slot per agent | `server/spec/inbox.feature:165-311`; `server/internal/inbox/slots.go` |
| Server: only leaders send `critical` or to `all` | `server/internal/messaging/service.go:57-58`, `server/internal/messaging/topics.go:79-80` |
| Send results are `{ok: true}`; post result `{postId}` | `server/TOOLS.md` (bus_send_direct, bus_add_post) |
| Docs that state "followUp, queued after the current turn" | `extension/README.md:116-119`, `server/internal/tools/ledger_tail.md.tmpl:37` (→ `server/TOOLS.md:530`), `server/spec/README.md:40`; tests `tests/format.test.ts:41-63`, `tests/index.test.ts:42-78` (the skill also says it, but the skill is retired) |

Effect of `followUp` today: agent-core keeps follow-ups until the agent **would stop** (`pi-agent-core/src/agent-loop.ts:1795-1823`), so a normal bus message to a busy agent waits until its whole run ends, which can be many tool calls later.

---

## 2. omp's native inter-agent messaging (18.12.6)

### 2.1 Surface (exact)

omp has **no dedicated message tool**. Messaging is the core `write` and `read` tools on the `agent://` URL scheme, plus the core `wait` tool. The code still calls it IRC (`IrcBus`, `irc:incoming`); a comment in `irc/bus.ts:359-362` still mentions an "irc send tool call" **[INFERENCE: a dedicated `irc` tool existed in earlier versions]**.

| Surface | Parameters | Description the model sees (verbatim) | Source |
|---|---|---|---|
| `write` to `agent://<id>` | `path: "agent://<id>"`, `content` | `` `agent://<id>`: latest completed output (status + progress while unpublished); nested IDs dotted, `/key/index` JSON path; write = message, `agent://all` broadcast only. `` | `prompts/internal-urls/agent.md`; handler `internal-urls/agent-protocol.ts:172-205` |
| `write` to `agent://all` | `content` | (same line) broadcast to every live peer visible to the sender | `irc/messaging.ts:57-74`; visibility `registry/agent-registry.ts:389-392` (running or idle, not advisors, not self) |
| `read agent://<id>[/<json-path>]` | path | latest output; status + progress while running | `internal-urls/agent-protocol.ts:207-232` |
| `wait` | none (`type({})`) | "Wait only when blocked with nothing else to do. Blocks on background jobs/services you started, or on the reply to a message you sent this run; returns on the first result, a message sent to you, or a steering interrupt; a safety cap returns a still-running snapshot. Nothing you started running and no reply owed? Returns at once. Results and messages auto-deliver. NEVER poll while work remains." | `prompts/tools/wait.md`; `tools/wait.ts:91-117` (`interruptible = true`, `approval = "read"`, `loadMode = "essential"`, cap 30 min `:24`) |
| Peer roster (subagents only) | — | "# Peers / Message peers via `write` with `path: "agent://<id>"` and `content` (broadcast: `agent://all`). Your id is `{{ircSelfId}}`. Currently visible peers: …" plus rules ("Use peer messages only for quick coordination…", "Idle/parked peers wake when messaged", "`write agent://<id>` never blocks", "Message Main only for questions, blockers, or decisions") | `prompts/system/subagent-system-prompt.md:27-49`; built at spawn `task/executor.ts:3960-3964` |
| Main agent hint | — | "Small missing detail? Run parallel; B messages A via `write agent://<id>`." | `prompts/system/system-prompt.md:179` |

Availability: subagents always; the main session only when it may spawn subagents (`irc/messaging.ts:16-21`, `internal-urls/agent-protocol.ts:177-184`, error "Peer messaging is unavailable in this session."). Errors: "A recipient is required.", "A non-empty message is required.", "Cannot send a message to yourself." (`irc/messaging.ts:52-56`), "agent://all is write-only; use it to broadcast a message." (`agent-protocol.ts:209`), unknown agent (`irc/bus.ts:137-142`). Approval tier of the message write: `"read"` (`agent-protocol.ts:149`).

Send result (receipt), `irc/messaging.ts:76-101`: "Delivered to X.", "Queued for X (was parked; revived).", "Failed: X is not running …"; broadcast: "Broadcast delivered to N of M peer(s)." with one line per recipient. Receipt outcomes `injected | woken | revived | failed` (`pi-tui/src/tools/irc.ts:33-37`).

### 2.2 Delivery semantics

`IrcBus.send` is fire-and-forget (`irc/bus.ts:66-94`). Routing in `#deliver` (`irc/bus.ts:135-218`), then `IrcBridge.deliver` in the recipient session (`session/irc-bridge.ts:191-260`):

| Recipient state | Sender is recipient's parent | Any other sender | Source |
|---|---|---|---|
| Blocked in `wait` (waiter pending) | message **is** the `wait` result; never injected | same | `irc/bus.ts:187-195` |
| Streaming (busy) | **steer**: user-role message "[Wait interrupted by message] `<irc from="parent" …>`" | queued in `#interrupts`, injected at the **next step boundary** without interrupting the tool batch; ends an interruptible `wait` | `irc-bridge.ts:230-245`; `prompts/steering/parent-irc.md`; boundary drain `session/agent-session.ts:2184-2217`; wait peek `pi-agent-core/src/agent-loop.ts:3378` |
| Idle | **wake turn** (any mode) | same | `irc-bridge.ts:258`; `agent-session.ts:1469-1564` |
| Idle in plan mode | folded into context, no turn | same | `irc-bridge.ts:247-257` |
| After a user Esc | peer `irc:incoming` records **still wake**; extension asides fold | | `agent-session.ts:1414-1428` |
| Parked / killed | revived from transcript, then delivered | same | `irc/bus.ts:153-185` |
| Hand-off failed | buffered in a process-local mailbox (cap 100, oldest dropped) for a later `wait` | | `irc/bus.ts:30-31, 72-76, 206-217, 311-326` |

What the model sees (`prompts/system/irc-incoming.md`, rendered at `irc-bridge.ts:206-215`):

```
<irc>
Incoming IRC message from agent `{{from}}`{{#if replyTo}} (reply to {{replyTo}}){{/if}}:
{{message}}
{{#if interrupting}}Sent while waiting/working. Active interruptible wait stopped early for immediate reading.{{/if}}
{{#if relayOnStop}}If response expected, reply via `write` (`path: "agent://{{from}}"`, `content: "…"`), when available; otherwise what you `yield` or say last this turn is delivered to `{{from}}` when you stop.{{else}}If response expected, reply via `write` (`path: "agent://{{from}}"`, `content: "…"`); may finish current step first. No one replies on your behalf.{{/if}}
</irc>
```

The body is escaped so it cannot close the envelope or forge a parent steer (`irc-bridge.ts:201-205`, `escapeHarnessTags`). An idle **subagent** woken by a direct message gets "relayOnStop": its final output of that turn is relayed back to the sender automatically (`irc-bridge.ts:196-200`); broadcasts and relays never trigger that (`pi-tui/src/tools/irc.ts:17-29`). The CHANGELOG shows this relay has needed several fixes (`CHANGELOG.md:146-148`).

### 2.3 Rendering

- Records are custom messages `customType: "irc:incoming"`, `display: true`, `details: {id, from, message, replyTo?, broadcast?, fromParent?}` (`irc-bridge.ts:206-228`).
- The TUI picks a card **by customType**, before any extension renderer: `irc:incoming | irc:autoreply | irc:relay | irc:workpool` → `buildIrcMessageCard` (`modes/utils/ui-helpers.ts:341-350`; `pi-tui/src/chat/chat-transcript-builder.ts:596-602`). Card: title "IRC ← from", quoted body collapsed to 3 lines, age (`pi-tui/src/tools/wait.ts:679-748`, `createIrcMessageCard`).
- Agent↔agent traffic that does not involve Main is mirrored to Main's UI as a display-only `irc:relay` card (`irc/bus.ts:357-383`).
- History previews: `[irc] from → me: …` (`session/session-history-format.ts:454-458`).
- The session event `irc_message` exists (`irc-bridge.ts:229`) but is **not** in the extension `pi.on` list (`extensibility/extensions/types.ts:1648-1718`), so extensions cannot observe native IRC directly.

### 2.4 Extension API relevant here

| API | What it gives | Source |
|---|---|---|
| `pi.sendMessage(msg, {deliverAs, triggerTurn})` with `deliverAs: "steer" \| "followUp" \| "nextTurn" \| "aside"` | **`aside`** = "injects the message at the next agent step boundary without interrupting the in-flight tool batch; when the session is idle it starts a turn regardless of `triggerTurn` (plan mode folds it into context instead)". It ends a running interruptible `wait`. It uses **the same queue as native peer messages** (`IrcBridge.queueAside`). `nextTurn` = hidden until the next turn. | `extensibility/extensions/types.ts:1845-1869`; `session/irc-bridge.ts:113-117`; `session/agent-session.ts:9678-9765` |
| `pi.sendUserMessage(content, {deliverAs})` | user prompt; `aside` also supported | `types.ts:1866-1869` |
| `pi.registerMessageRenderer(customType, renderer)` | custom TUI component per customType (used only if omp has no built-in card for that type) | `types.ts:1827-1828`; `ui-helpers.ts:370-374` |
| `pi.registerTool({loadMode, approval, renderCall, renderResult, defaultInactive, …})` | extension tools **default to `loadMode: "discoverable"`** (moved out of the top-level schema to `xd://` devices or tool search) and **approval `"exec"`** | `types.ts:884-964` (`:898`, `:904-906`); `pi-agent-core/src/types.ts:1045-1054` |
| `pi.setActiveTools / getActiveTools` | already used | `extension/lib/activation.ts` |
| `pi.on("before_agent_start")` → `systemPrompt: string[]` | add a roster/guidance block per request | `types.ts:1562-1565` |
| `pi.on("message_end")`, `turn_start`, `agent_end`, … | observe when an injected message entered the context **[INFERENCE: injected asides emit message events; `agent-loop.ts:1337-1343` pushes them into the turn's messages, which the loop emits]** | `types.ts:1684-1691` |
| `ctx.ui.setStatus / setWidget / notify`, `ctx.isIdle()` | status line, widget above the editor, idle check | `types.ts:339-352, 697` |
| `pi.events` (EventBus) | in-process pub/sub between the extensions bound to one session (and omp's own `task:subagent:*`, `lsp:startup`, `mcp:connection-status` channels); a subagent session gets its own bus (§2.6) | `types.ts:1963-1964`; `utils/event-bus.ts:3-33` |
| `ctx.agent` | `{kind: "main" \| "sub", id, name, depth, parentId?}`, e.g. id `"Main"` or `"0-Explore"` | `types.ts:520-537` |

Upstream pi: `sendMessage` accepts only `"steer" | "followUp" | "nextTurn"` (`badlogic/pi-mono packages/coding-agent/src/core/extensions/types.ts`, lines 445/1737/2109 on `main`); there is no `aside`, and no built-in subagent messaging (subagents are an example extension, `examples/extensions/README.md:44`). Its docs list `pi.events` for "Communicate with another extension" (`docs/extensions.md:186`).

### 2.5 `pi.events`: could it be the mailbox?

No. Evaluated against what a mailbox needs:

| Need | `pi.events` | Verdict |
|---|---|---|
| Durable across disconnects/restarts | in-memory `Map` of listeners, no storage (`utils/event-bus.ts:3-33`) | no |
| Cross-process / cross-machine | one instance per main session (`main.ts:554, 2377`); not even shared with subagents (§2.6) | no |
| Gets content into the model's context | no; only `sendMessage`/`sendUserMessage` do | no |
| Ordering, ack, backpressure | synchronous fan-out; handler errors are caught and logged | no |

Where it **is** useful (`omp-events.feature`):
1. Publish every delivered entry and connection state (`agent-bus:entry`, `agent-bus:connection`) so other extensions (status widgets, notifiers, a dashboard) can react without a second inbox stream, which the server would refuse anyway (one slot per agent).
2. A handshake on `agent-bus:instance` so that two loaded copies (installed plugin + `-e` working tree, `.omp/AGENTS.md:115-120`) do not both register tools and fight over credentials. **[INFERENCE: both copies get the same bus because `loadExtensions` binds all extensions of one load to one `resolvedEventBus`, `extensibility/extensions/loader.ts:503-512`; verify that `-e` and installed plugins load in the same call.]**
3. Not useful for subagents: a subagent session gets a **new** EventBus (`task/executor.ts:4611-4678` passes none, so `sdk.ts:1773` creates one), so nothing on `pi.events` crosses from the main session to a subagent. Subagents talk to the main session via native `write agent://`.

### 2.6 How omp subagents get extension tools (and how to withhold them)

`.omp/AGENTS.md:130` says "`filterActiveTools` drops parent-owned tools" and subagents "only get the tools their parent passes on". In 18.12.6 that is not how extension tools reach a subagent:

| Step | What omp does | Source |
|---|---|---|
| 1. Extension binding | Each subagent session **re-binds the parent's extension modules to its own `ExtensionAPI`** ("re-bind their factories to THIS session's ExtensionAPI without evaluating the same module graph again"). The parent's extension instance and its registered tools are not shared. | `sdk.ts:2759-2790`; options from `task/executor.ts:4647-4650` (`preloadedExtensionPaths`, `preloadedPreparedExtensions`) |
| 2. Lifecycle | The subagent runs `initializeExtensions` and emits its own `session_start`; `ctx.agent.kind` is `"sub"`. | `task/executor.ts:4878-4888`; `modes/runtime-init.ts:212`; `types.ts:520-537` |
| 3. Admission | Extension-registered tools are **always included** unless `defaultInactive`/`hidden`, even when the agent definition lists explicit `tools`. A tool registered later (our `registerRemoteTools` runs after `listTools`) is enabled on registration under the same rule. | `sdk.ts:4235-4248`; `sdk.ts:5013-5045` |
| 4. `filterActiveTools` | Wraps the subagent's `setActiveTools`; it only strips the parent-owned `todo` (kept under prewalk). | `task/executor.ts:4784-4787, 4817-4821, 4886`; `modes/runtime-init.ts:127-128` |
| 5. Restricted subagents | `restrictToolNames` sessions (e.g. seance) bind no extension tools at all. | `sdk.ts:3546, 5062`; `task/executor.ts:4239` |
| 6. Credentials | The subagent's instance resolves credentials itself (see §1). Module state is shared (same module graph), instance state is not. **[INFERENCE from sdk.ts:2759-2761]** | `extension/index.ts:192-206` |

Consequences for the ruling:
- **The subagent's own extension instance decides what the subagent gets.** Withholding must happen at **registration**: a registered non-inactive tool is switched on by omp before the extension can switch it off (step 3). Today `registerRemoteTools` excludes only `INBOX_TOOLS` in subagents (`extension/lib/tools.ts:17`, `extension/lib/connection.ts:56`), and `restart()` activates `bus_spawn_worker` there too (`connection.ts:48, 58`).
- Target (`omp-subagents.feature`): in a session with `ctx.agent.kind === "sub"`, `registerRemoteTools` registers only an **allow-list** `SUBAGENT_TOOLS = [bus_list_blackboards, bus_list_threads, bus_read_thread, bus_create_thread, bus_add_post]`. An allow-list, not a block-list, so a tool the server adds later stays with the main session. `bus_spawn_worker` stays registered (it is registered at load, before the kind is known, `extension/index.ts:202`) but is `defaultInactive` and never activated in a subagent; `localTools` becomes `[]` there. The local `bus_wait` is not registered in subagents.
- **Identity.** Subagents must act as the main session's active identity or not at all. The accepted-invite case already works (root-session key). Env and `/bus promote` credentials do not reach the subagent's instance, which then falls back to the stored token, possibly **another agent**, or to nothing. Proposal: the main session's instance publishes its active credentials in a module-level map keyed by the root session key (`extension/lib/session-store.ts` already computes it), and a subagent instance reads only that map, never the store or the env. It is the only channel: `pi.events` is per session (§2.5), env is deleted, and writing a promoted token to disk contradicts "never stored" (`extension/lib/credentials.ts:13-15`). This is shared mutable state in `extension/lib/`, so the architecture rules require asking first (`.omp/AGENTS.md:54`).
- **Attribution.** The server records the token's agent as author, so a subagent's posts show the main session's bus name. Without a marker, readers cannot tell which of several parallel subagents wrote a post, or whether the main session itself did. Recommendation: **the extension adds it deterministically**. In a subagent, the local wrappers of `bus_add_post` and `bus_create_thread` prefix the body with `[via subagent <ctx.agent.id>]\n`. A convention the model has to remember would be forgotten. A server field (`via`) would need a migration and API change; defer it until the web frontend needs structured attribution (Q12). Side effect to know: authors get no notification for their own posts, so the main session is **not** notified of its subagents' posts. It learns about them from the subagent's result or `agent://` messages.
- `.omp/AGENTS.md:130` should be corrected when this is implemented (finding for the postponed-findings list).

---

## 3. What "behave almost exactly like the regular message tools" can mean

| Level | Meaning | Feasible? |
|---|---|---|
| L1 Delivery | bus mail arrives when and how native peer messages arrive (step boundary, steer only for `critical`, wake when idle, fold in plan mode, ends `wait`) | **Yes, cleanly**: `deliverAs: "aside"` is the documented extension handle on the native queue. |
| L2 Look | bus mail renders like an IRC card; the model gets a fenced envelope with a reply instruction | **Yes**: structured `details` + `registerMessageRenderer`; envelope in our own text. |
| L3 Affordances | send is one top-level call with a receipt, no approval prompt; `wait` for a reply; who-is-there roster | **Mostly**: `loadMode`, `approval`, `renderCall`; `bus_wait` (shared server contract, shadowed locally in omp); receipts/presence need a small server change; roster block overlaps guidance. |
| L4 Same idiom/namespace | `write agent://<bus-agent>` reaches a remote agent; remote agents appear in omp's roster and Agent Hub | **Only through omp internals** (see S6/S7). Not recommended now. |

Recommendation: deliver L1–L3; treat L4 as a later option that needs a public omp API.

---

## 4. Gap table: bus vs native

| # | Aspect | omp native | Bus today | Gap → target |
|---|---|---|---|---|
| G1 | Send idiom | `write agent://<id>` (`agent-protocol.ts:172-205`) | `bus_send_direct {space,to,body,critical?}`, `bus_send_topic` | Different by design (space, permissions, MCP parity). Keep names (trade T1). |
| G2 | Non-urgent message to a busy agent | step-boundary injection (`irc-bridge.ts:243`) | `followUp`, after the whole run (`deliver.ts:18`) | **Main gap of the TODO** → `aside` (S1). |
| G3 | Urgent message | parent→busy child = steer (`irc-bridge.ts:231-241`) | `critical` (leaders only) = steer | Equivalent. **PO ruling: only `critical` steers**; a leader's normal message is an aside like any other. |
| G4 | Idle recipient | woken in every mode (`irc-bridge.ts:258`) | woken only in tui/rpc (`deliver.ts:18`) | Native is not safe for one-shot modes; keep the mode gate there (R1). |
| G5 | Plan mode | folded, no turn (`irc-bridge.ts:247-257`) | `triggerTurn: true` starts a turn [INFERENCE: omp may still guard it] | `aside` folds automatically (`agent-session.ts:9730-9737`). |
| G6 | After user Esc | peer messages wake (`agent-session.ts:1414-1428`) | queued follow-up | `aside` folds instead of waking; differs from native (Q3). |
| G7 | Waiting for a reply | `wait` blocks on owed reply; waiter consumes the message directly (`tools/wait.ts:53-117`, `irc/bus.ts:187-195`) | nothing; `wait` returns at once ("no reply owed") | `bus_wait` (S5): one contract for all clients (mail BA), shadowed locally in omp. |
| G8 | Receipt | Delivered / Queued (revived) / Failed (`messaging.ts:91-101`) | `{ok: true}` | Server receipts `live`/`queued` (S9, `presence.feature`). |
| G9 | Roster / presence | peers with kind, status, activity in subagent prompt (`subagent-system-prompt.md:27-49`) | `bus_list_participants`: roles only | `connected` flag (S9) + optional prompt block (S13, guidance overlap). |
| G10 | Envelope | `<irc>` with reply instruction; body escaped (`irc-bridge.ts:201-215`) | header line + raw body (`format.ts:21-23`); a body can forge a second `[agent-bus] … · CRITICAL` header | Fence + escape + reply hint (S3). |
| G11 | Transcript | IRC card (`ui-helpers.ts:341-350`) | generic custom component, no details | Renderer + details (S2). |
| G12 | Observing others' traffic | `irc:relay` cards on Main (`irc/bus.ts:357-383`) | none | Out of scope (web frontend BA). |
| G13 | Durability | process-local, mailbox only for failed hand-offs, cap 100 | SQLite inbox, replay of unacked entries (`inbox.feature:249-257`) | Bus is stronger; but ack-before-context loses entries (R2) → ack on consumption (S10). |
| G14 | Tool presentation | `write`/`wait` are core, essential; message write tier `read` | extension tools default `discoverable` + approval `exec` (`types.ts:898, 904-906`); not set in `tools.ts:30-43` | Set per tool (S4). |
| G15 | Auto-reply of a woken idle agent | relayOnStop for subagents (`irc-bridge.ts:196-200`) | none | Do not copy (trade T4). |
| G16 | Broadcast | `agent://all`, anyone | topic `all`, leaders only | Keep bus permissions (trade T3). |
| G17 | Shared state | none; nearest: `local://` artifacts, `read agent://` outputs | blackboards/threads | Bus advantage; surface it better (S11). |
| G18 | Revival of stopped agents | parked agents revived on message (`irc/bus.ts:153-185`) | offline agents read the backlog on reconnect | Different by nature (remote processes). `bus_spawn_worker` covers starting. |
| G19 | Subagents | message their parent/peers natively (`write agent://`); roster in prompt | every bus tool except inbox, plus `bus_spawn_worker` (`connection.ts:48, 56-58`); identity may differ from the main session's (§1) | **PO ruling**: blackboard read/post only (allow-list at registration), main-session identity, attribution line (S14-S16, `omp-subagents.feature`). |

---

## 5. Candidate seams

Ratings 1 (poor) to 5 (best). *Clean* = fits documented APIs and the repo's architecture; *Simple* = small change, few moving parts; *Powerful* = how much of the PO's ask it delivers.

| Seam | What | Clean | Simple | Powerful | Justification |
|---|---|---|---|---|---|
| **S1** aside delivery | `deliveryOptions`: non-critical → `aside`, critical → `steer`; one-shot modes: `aside` only while busy (`ctx.isIdle()`), else no turn | 5 | 5 | 4 | Documented extension option that shares omp's native peer queue (`irc-bridge.ts:113-117`). Delivers the TODO in one function (`deliver.ts:17-19`). Plan-mode fold and wait-interrupt come for free. |
| **S2** details + renderer | put the entry in `details`; `registerMessageRenderer("agent-bus", …)` drawing an IRC-style card (own component, or `createIrcMessageCard` from `@oh-my-pi/pi-tui/tools/wait`) | 4 | 4 | 2 | Public API. Reusing omp's card function is a deep import (a bundled module key in `dist/cli.js`, but no stability promise); an own component avoids that. Cosmetic but visible. |
| S2b borrow `customType: "irc:incoming"` | send bus entries as native IRC records | 1 | 5 | 3 | Gets the native card, post-Esc wake and `wait` drain for free, but `wait` would turn them into IRC results and the native envelope tells the model to reply via `write agent://<from>`, which fails for bus agents. **Reject.** |
| **S3** envelope | fence each entry, escape the body's markers, add a reply hint per entry type | 5 | 5 | 3 | Pure formatting in `format.ts`; closes a forgery hole (members could fake a leader's CRITICAL line today). |
| **S4** tool presentation | per tool: `loadMode: "essential"` for the messaging core; `approval` `read`/`write`; `renderCall`/`renderResult` for sends and thread reads | 4 | 4 | 3 | Public `ToolDefinition` fields. Where the metadata lives is a choice: (a) table in the extension next to `INBOX_TOOLS`; (b) server tool metadata (`readOnly`/`destructive`) exposed in `GET /v1/tools` and as MCP annotations, which also helps MCP clients. Recommend (b) for approval, (a) for `loadMode` (omp-only). |
| **S5** `bus_wait` | **one** long-poll contract for every client: `bus_wait {timeoutSeconds 1..300 (default 30), from?, threadId?, after?}` → `[entries]` (`[]` on timeout), defined in the server registry (no ack there). In omp the extension excludes the server tool and registers a local one with the same name/schema/result; it takes entries from the stream dispatcher (not injected again) and they count for the ack watermark | 4 | 3 | 4 | Mirrors the native waiter. One name and shape in TOOLS.md, MCP and omp. Needs an internal dispatcher between `inbox.ts` and `deliver.ts`. Contract owned by the mail BA (§6.3). |
| **S6** `bus://` scheme | register a protocol handler in omp's process-global `InternalUrlRouter`: `write bus://alpha/peer`, `read bus://alpha/board/plan/7` | 2 | 3 | 5 | Exactly the native idiom; tier `read`; its one-line doc lands in the system prompt (`system-prompt.ts:970`). But the router is not part of `ExtensionAPI`; it needs `import "@oh-my-pi/pi-coding-agent/internal-urls"` (exported via `"./*"`, and a bundled key in `dist/cli.js`), and the router is process-wide, so subagents would also write as the parent. omp's own precedent is the RPC host bridge (`modes/rpc/host-uris.ts:100-126`). Option for later, ideally after asking omp for a public `registerUrlScheme`. |
| S7 registry bridge | register remote bus agents as `AgentRegistry` refs with a proxy "session" so `write agent://<name>` routes to the bus and they show in Agent Hub | 1 | 1 | 5 | `AgentRef.session` is a full `AgentSession` (`registry/agent-registry.ts:66-83`); IrcBus, `wait`, Agent Hub (kill/revive/chat), persisted rosters and the lifecycle manager all touch it. Flat ids collide with local agents; spaces don't fit `agent://<id>`. **Reject.** |
| **S8** `pi.events` | publish entries/connection; two-copy handshake | 5 | 5 | 2 | Public, trivial, decoupled; not a mailbox (2.5). |
| **S9** server receipts + presence | send/post results gain `receipts: [{to, delivery: live\|queued}]`; participants gain `connected` | 4 | 3 | 4 | The inbox service already knows slot holders (`internal/inbox/slots.go`). Messaging must ask it through a service method (features own their tables; no table involved: slots are in memory). Additive result change; spec + TOOLS.md regen. |
| **S10** ack on consumption | ack when the injected message emits `message_end` (or when `bus_wait` returns), not when queued | 4 | 3 | 4 | Fixes silent loss: an aside is dropped on a session-generation change (`agent-session.ts:9687, 9729`) and a follow-up can be stranded when a one-shot run ends. **[INFERENCE: event timing must be verified in a smoke test.]** Overlaps mail BA. |
| **S11** blackboard notices | compact `thread_post` notice (full if short, else head + "read with bus_read_thread"); coalesce per thread before injection; `nextTurn` for lifecycle notices; status line with unread counts | 4 | 3 | 4 | Keeps 64 KiB posts out of the context; the blackboard stays the history. Uses public APIs only. |
| S12 auto-relay | after a bus-woken turn, send the agent's last text back to the sender | 2 | 3 | 2 | Native relay has had repeated bugs (`CHANGELOG.md:146-148`); with topics and posts the "sender" is ambiguous. **Reject** (trade T4). |
| ~~S13~~ roster block | ~~`before_agent_start` → system prompt block "Bus peers: name, role, connected"~~ | — | — | — | **Dropped (round 3).** `before_agent_start` misses bus-triggered turns. The guidance BA's `context`-event block (`docs/design/specs/guidance/session-context.feature`) is the roster. Presence stays in `bus_list_participants`; putting `connected` into the block would break the prompt cache. |
| **S14** subagent allow-list | in `ctx.agent.kind === "sub"`: register only `SUBAGENT_TOOLS` (5 blackboard tools), never activate `bus_spawn_worker`, no `bus_wait` | 5 | 5 | 4 | Uses the only seam omp offers (the subagent's own extension instance, §2.6). Registration-time, because omp enables registered tools by itself (`sdk.ts:5013-5045`). Allow-list keeps future server tools main-only. |
| **S15** subagent identity | the main instance publishes its active credentials in a module-level map keyed by root session; subagent instances read only that | 3 | 4 | 4 | Fixes env/promote identities not reaching subagents and the fallback to a different stored token. Module-level shared state in `extension/lib/` needs approval (`.omp/AGENTS.md:54`); relies on one module graph per process **[INFERENCE: sdk.ts:2759-2761]**. Alternatives are worse: `pi.events` is per session, env is deleted, a promoted token must not be stored. |
| **S16** attribution line | subagent wrappers of `bus_add_post`/`bus_create_thread` prefix `[via subagent <ctx.agent.id>]` | 4 | 5 | 3 | Deterministic, no server change. A server `via` field is cleaner data but needs a migration; defer (Q12). |

---

## 6. Recommendation (with the round 2 rulings)

### 6.1 Phases

**Phase 1, extension only (small diffs):**
1. **S1** (ruling 1): only `critical` → `steer`; every other entry → `aside`. Interactive modes: `aside` also wakes idle sessions (no `triggerTurn` needed). `print`/`json`/unknown: `aside` only while `!ctx.isIdle()`, otherwise `nextTurn`: no turn starts, and the entry becomes part of the next model request, e.g. the one-shot run's first request (`omp-delivery.feature`, one-shot rule). Lifecycle notices (`topic_closed`, `thread_removed`, `blackboard_removed`) → `nextTurn` (proposal, Q5: they are notices, not messages). A `nextTurn` entry counts as consumed for the ack watermark when a model request contains it. Update `extension/README.md:116-119`, `ledger_tail.md.tmpl:37` (+ regenerate `TOOLS.md`), `server/spec/README.md:40`, tests `tests/format.test.ts`, `tests/index.test.ts`.
2. **S14** (ruling 2): subagent allow-list at registration; `bus_spawn_worker` never activated in subagents. Correct `.omp/AGENTS.md:130`.
3. **S16**: attribution line for subagent posts and threads.
4. **S3 + S2**: entry in `details`; fenced, escaped envelope with a reply hint; an `agent-bus` renderer with an own card component (no deep import).
5. **S4** (ruling 3): `loadMode` per §6.2; approval tiers per `omp-tools.feature` once the PO answers Q2; withhold `bus_inbox_ack` from the model in omp.
6. **S8**: publish on `pi.events` (cheap; enables widgets and the two-copy fix).

**Phase 2:**
7. **S15** subagent identity via a module-level map (needs approval for shared state in `extension/lib/`).
8. **S10** ack watermark (mail BA's contract, `docs/design/specs/mail/omp-mail-delivery.feature`).
9. **S5** `bus_wait`: server tool (mail BA) + local shadow in omp (ruling 5, §6.3).
10. **S9** receipts and presence on the server; then receipt texts and the outgoing card.
11. **S11** blackboard notices and status line.

**Later / ask upstream:** S6 `bus://` (ask omp for a public URL-scheme API first; subagents limited to board paths, `omp-bus-url.feature`). S13 is dropped in favour of the guidance `context` block.

Trade-offs: messages now reach busy agents mid-run, which is what the TODO wants but can distract an agent more than end-of-run follow-ups did (mitigated by coalescing and compact post notices, R5). The extension becomes omp-specific in delivery (`aside` does not exist in upstream pi, R3). We do not get L4 (same namespace), so the main session uses two idioms: `write agent://` for its subagents and `bus_*` for the bus. Subagents use only one bus idiom: the blackboard.

### 6.2 Essential tools (ruling 3)

Main session, `loadMode: "essential"` (5 tools):

| Tool | Why top-level |
|---|---|
| `bus_send_direct` | the core "message an agent" call, the analogue of `write agent://<id>` |
| `bus_send_topic` | workstream messages; leaders' `all` broadcasts |
| `bus_wait` | the analogue of native `wait`; needed exactly when the agent is otherwise blocked |
| `bus_read_thread` | blackboard notices point here ("read the full thread"); must be one call away |
| `bus_add_post` | answering on a thread is how shared state is written |

Everything else stays `discoverable`: set-up and administration (`bus_whoami`, spaces, invites, revoke, `bus_mint_leader`, `bus_spawn_worker`), topic and thread management (create/close/subscribe/unsubscribe/remove), listings (`bus_list_*`), board creation/deletion, and `bus_inbox_read` (the stream delivers mail; `bus_wait` covers blocking). `bus_inbox_ack` is **not registered** in omp: the extension owns the cumulative ack watermark, and a manual ack could delete entries still queued as asides.

Subagents: the same per-tool `loadMode` applies to their 5 tools, so `bus_read_thread` and `bus_add_post` are essential there too, and `bus_list_blackboards`, `bus_list_threads`, `bus_create_thread` are discoverable. Discoverable tools are mounted under `xd://` only when the session has `read` and `write` (`sdk.ts:5027-5034`); otherwise they go to tool search.

### 6.3 One long-poll seam (ruling 5)

**Decision: `bus_wait`, with the mail BA's contract** (agreed with BA-mail; their write-up is `docs/design/specs/mail/analysis.md` §5):
`bus_wait {timeoutSeconds 1..300 (default 30), from?, threadId?, after?}` → `[entries]`, `[]` on timeout.

- Defined once in the **server registry**, so MCP and HTTP clients get it and `TOOLS.md` documents it. The server version does **not** ack.
- The mail BA first proposed `bus_inbox_read {waitSeconds}`. Its rationale (works in every client, no transport change) holds for a server-side tool. But in omp `bus_inbox_read` would race the live SSE stream: the same entries would arrive as a tool result and as asides. With a separate name, omp can shadow exactly that one tool: the extension excludes the server's `bus_wait` from `registerRemoteTools` (like `bus_spawn_worker`) and registers a local tool with the same name, schema and result. The local tool takes entries from the stream dispatcher (never injected twice) and they count toward the ack watermark. Without a live stream (another session holds the slot) it forwards to the server tool and does not ack.
- `bus_inbox_read` keeps its non-blocking contract.
- Main session only (ruling 2).

---

## 7. Requirement trades (explicit)

| # | Literal requirement | Proposed instead | Why it is a good trade |
|---|---|---|---|
| T1 | "same tools as the regular message tools" | keep `bus_*` names and shapes; match **delivery, look and affordances** (L1–L3) | omp has no message tool to copy (it is `write agent://`). Keeping `bus_*` keeps one tool registry for HTTP, MCP and omp (`.omp/AGENTS.md` architecture) and keeps `TOOLS.md` true. |
| T2 | "remote agents behave like local peers" (`agent://`) | separate namespace (`bus_*` now, `bus://` later) | Avoids faking `AgentSession` (S7). A separate name signals different guarantees: durable, permissioned, cross-machine, space-scoped. |
| T3 | native permissions (anyone broadcasts; parent always steers) | bus permissions stay: only leaders send `critical` or to `all`; only `critical` steers (ruling 1) | Parity of mechanism, not of policy. `critical` ≈ parent steer. |
| T4 | native auto-reply of a woken agent's last output | explicit replies + `bus_wait` | Native relay is fragile; on topics/threads the reply target is ambiguous. |
| T5 | "straight into the input queue" = `followUp`/`steer` | `aside` (step boundary) for normal, `steer` for critical | `followUp` waits for the whole run; `aside` is what omp itself uses for peer messages. |
| T6 | every entry delivered in full | thread posts as compact notices; lifecycle notices without wake-up | The blackboard is the history; the context should only learn what changed. |
| T7 | ack right after delivery | contiguous ack watermark over consumed entries | Turns "handed to omp" into "seen by the model"; uses the server's replay instead of losing entries. |
| T8 | subagent posts "show the parent's bus name" (fine as is) | plus an automatic `[via subagent <id>]` line | Readers can tell parallel subagents and the main session apart without a server change; a structured `via` field can follow later. |
| T9 | the model may ack its inbox (`bus_inbox_ack`) | the extension alone acks in omp | A cumulative manual ack can delete queued, unseen entries. Non-omp clients keep the tool. |

---

## 8. Blackboards in omp

omp has no native equivalent. Closest relatives are documents read on demand: `local://` shared artifacts and `read agent://<id>` outputs. So the blackboard should feel like a **document you read**, with **short notices** when it changes:

- **Notices** (`omp-blackboards.feature`): `thread_post` as a compact card (board/thread/title/author, first lines, "read the full thread with `bus_read_thread`"), coalesced per thread while waiting for the next step boundary; critical posts steer in full; `thread_removed`/`blackboard_removed`/`topic_closed` as `nextTurn` (no wake-up).
- **Reading**: `renderResult` for `bus_read_thread` as a thread document (title, posts with author/time, newest expanded).
- **Ambient awareness**: `ctx.ui.setStatus("agent-bus", "bus: 2 new posts in plan/7")`, cleared on `bus_read_thread` or logout; counts kept client-side from delivered entries (the server tracks subscriptions, not read state).
- **Subagents** (ruling 2) are blackboard-only participants: they list boards and threads, read threads, create threads and post. They cannot create or delete boards, remove threads or (un)subscribe, and they have no inbox. Their posts carry `[via subagent <id>]`, and the main session is not notified of them (authors are never notified). **But** `bus_create_thread` auto-subscribes the creator (`server/TOOLS.md`, bus_create_thread), which is the main identity. So a thread a subagent creates subscribes the main session, and the main session gets notices for other agents' later posts there. That is intended: the main session can unsubscribe with `bus_unsubscribe_thread`.
- **Later**: `read bus://alpha/board/plan/7` (S6) would make threads first-class documents with line selectors.
- Not proposed: server-side read markers per agent (new table/migration) until the PO asks for unread counts across sessions.

---

## 9. Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | `aside` to an **idle** session starts a turn regardless of `triggerTurn` (`types.ts:1857-1859`); in `print`/`json` it could race the CLI's own prompt (the reason for today's gate, `deliver.ts:11-16`). **[INFERENCE: whether `#admitSubmission` serializes this was not verified.]** | Keep the mode gate; use `aside` in one-shot modes only while busy. |
| R2 | An `aside` is silently dropped when the session generation changes (`/new`, `/resume`, switch) between send and injection (`agent-session.ts:9687, 9729`); today's ack-at-send would lose the entry. | Ack watermark (S10); the stream restarts on switch anyway and replays. |
| R3 | Upstream pi has no `aside`; an unknown `deliverAs` might fall back to the host's default (`steer` in pi) and interrupt. **[INFERENCE]** | The extension targets omp (`package.json` keyword `omp-plugin`, imports `@oh-my-pi/*`); document omp ≥ the version with `aside`, or detect the host and fall back to `followUp`. |
| R4 | Deep imports (`@oh-my-pi/pi-tui/tools/wait`, `@oh-my-pi/pi-coding-agent/internal-urls`) are internal. | Own card component in phase 1; S6 only with a public API. |
| R5 | Mid-run injection distracts agents more than end-of-run follow-ups. | Compact/coalesced notices; guidance on topic subscriptions (guidance BA). |
| R6 | Body forgery of headers/critical markers (exists today). | S3. |
| R7 | `approval: "read"` on sends lets an agent message others without asking the user (native `write agent://` behaves the same). | PO decision Q2; destructive tools stay `write`. |
| R8 | "connected" = stream slot held, not "attentive" (an idle print-mode session holds a slot and never acts). | Name it `connected`, not `online`/`active`; document. |
| R9 | omp's IRC and subagent tool semantics change often (`CHANGELOG.md:144-148, 365, 829`). | Prefer documented `ExtensionAPI` options over mirroring internals; smoke test per omp upgrade, including "which `bus_*` tools does a subagent see". |
| R10 | Essential tools add their schemas to every request. | 5 essential tools in the main session (§6.2), 2 in subagents. |
| R11 | Until S15 lands, a subagent of a promoted or env-credentialed main session acts as the **stored** agent (another identity) or has no bus tools. | Phase-1 stop-gap: in a subagent, register the blackboard tools only when the resolved identity comes from the accepted invite (root-keyed); otherwise register none and log why. |
| R12 | Extension tools are admitted even to agents whose definition lists only read-only tools (`sdk.ts:4235-4243`), so a read-only scout also gets `bus_add_post`/`bus_create_thread`. | Accepted: the PO ruling fixes the five tools for every subagent (Q11 closed). |
| R13 | Module-level credential map (S15) is shared mutable state across sessions of one process. | Key strictly by root session; clear on the main session's `session_shutdown`/logout; ask first (`.omp/AGENTS.md:54`). |

---

## 10. Open questions for the PO

1. ~~**Q1 Essential tools.**~~ *Closed by the PO ruling:* the five in §6.2 (`bus_send_direct`, `bus_send_topic`, `bus_wait`, `bus_read_thread`, `bus_add_post`), the rest discoverable, and `bus_inbox_ack` not offered to the model in omp.
2. ~~**Q2 Approval.**~~ *Decided (PO-2):* reads, sends, posts and thread creation `read`; management `write`; `bus_spawn_worker` `exec` (`omp-tools.feature`).
3. **Q3 After Esc.** When the user interrupted the agent, should bus mail restart it (native peer messages do) or wait for the user (omp's rule for extension asides)? Proposal: wait for the user; critical still steers the next turn.
4. ~~**Q5 Lifecycle notices.**~~ *Decided (PO-5):* delivered on the next turn without waking the agent, generalised as the server-assigned delivery class `deferred` (`interrupt` | `queue` | `deferred`, `docs/design/specs/mail/delivery-class.feature`).
5. **Q6 Post notices.** Truncation size for post notices (proposal: 20 lines / 2 KiB) and coalescing per thread: OK?
6. **Q7 Auto-reply.** Confirm we do not copy omp's relay of a woken agent's last output.
7. **Q8 `bus://`.** Worth asking omp upstream for a public URL-scheme API so the bus can offer `write bus://…`/`read bus://…`?
8. ~~**Q9 Presence.**~~ *Decided (PO-9):* approved; `connected` includes agents blocked in a server `bus_wait`.
9. ~~**Q10 Upstream pi.**~~ *Decided (PO-6):* omp only; the `followUp` fallback is removed.
10. ~~**Q11 Read-only subagents.**~~ *Closed by the PO ruling:* every omp subagent gets the five blackboard tools, read-only agents included.
11. **Q12 Attribution.** Is the automatic `[via subagent <id>]` body prefix enough, or do you want a structured `via` field on posts (a server migration numbered after `0001_schema`, never folded into it; `docs/design/specs/REVIEW.md` §11)?
12. ~~**Q13 Subagent identity.**~~ *Decided (PO-8):* approved; the module-level map keyed by the root session (S15).

(Q4 about leaders' normal messages steering was settled by ruling 1 and is removed.)

---

## 11. Overlaps with the other BAs

| Topic | Other BA | Note |
|---|---|---|
| Long-poll seam | mail (`docs/design/specs/mail/`) | **Settled**: one contract `bus_wait` (mail BA owns the server spec); omp shadows it locally (§6.3). |
| Ack timing | mail | **Settled**: contiguous watermark (mail BA's `omp-mail-delivery.feature`); `omp-delivery.feature` follows it. |
| "You have mail" notice vs injecting each entry (TODO 35 vs 36) | mail | For omp: inject via `aside` (TODO 36, ruling 1). The edge-triggered notice fits MCP/non-omp clients. |
| MCP push | mail | Receipts/presence (S9) are transport-neutral and help MCP clients too. |
| Reply hints in envelopes, roster block, delivery wording in agent-facing docs | guidance (`docs/design/specs/guidance/`) | The guidance BA owns the replacement for the retired skill; it should state the aside/steer rule, the subagent blackboard-only rule and the attribution line. TODO 28 (spawn `task` as a bus DM from the leader) fits S2/S3. |
| Observing others' traffic (native `irc:relay`) | web frontend | Not proposed for the extension. A structured `via` field (Q12) would help the frontend. |
| Duplicate extension copies; stale `.omp/AGENTS.md:130` | postponed findings | `pi.events` handshake (`omp-events.feature`) is one fix; correct the subagent-tools note in AGENTS.md. |

---

## 12. Summary

- omp's "regular message tools" are not tools: they are `write`/`read` on `agent://` plus `wait`. Copying names is neither possible nor useful; copying **behaviour, look and affordances** is.
- Ruling 1 maps 1:1 onto a documented extension option: **`deliverAs: "aside"`** for every normal message (next step boundary, no interruption, wakes idle sessions, folds in plan mode, ends `wait`); only `critical` steers.
- Ruling 2: omp gives a subagent whatever **its own extension instance** registers (non-inactive tools are switched on automatically; `filterActiveTools` only strips `todo`). So the extension must use an allow-list **at registration** in `ctx.agent.kind === "sub"`: `bus_list_blackboards`, `bus_list_threads`, `bus_read_thread`, `bus_create_thread`, `bus_add_post`. Subagents must act as the main session's identity: today they lose env and promote credentials and may fall back to another stored identity (S15). Their posts get an automatic `[via subagent <id>]` line.
- Ruling 3: essential tools in the main session are `bus_send_direct`, `bus_send_topic`, `bus_wait`, `bus_read_thread` and `bus_add_post`; `bus_inbox_ack` is not offered to the model in omp.
- Ruling 5: one long-poll seam, `bus_wait` (mail BA's contract, server-defined, shadowed locally in omp so stream entries are never delivered twice).
- `pi.events` is not a mailbox and does not even reach subagents (each subagent session has its own bus). It is still a good fan-out and a fix for two loaded copies.
- Plugging into omp's native namespace (`agent://` peers, Agent Hub) needs omp internals; reject the registry bridge, and keep `bus://` as a later option behind a public omp API.
