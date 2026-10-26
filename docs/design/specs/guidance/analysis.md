# Agent guidance and onboarding: analysis

Scope: `tmp/TODO.md:16-29` ("Agent-facing docs and onboarding"), plus the product owner's question "is a skill really the right fit? How does the regular tooling do it?". Nothing in the repo was changed; all output is in `docs/design/specs/guidance/`.

Notation: `CA/` = `node_modules/@oh-my-pi/pi-coding-agent/` (omp 18.12.6, the repo's devDependency and the same version installed globally at `C:\Users\tillm\node_modules\@oh-my-pi\pi-coding-agent`). `AC/` = `node_modules/@oh-my-pi/pi-agent-core/`. `SDK/` = `~/go/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.8.0/`. Unverified statements are marked [INFERENCE].

## 1. Summary of recommendations

| # | Item | Recommended seam | Where |
|---|---|---|---|
| 1 | Doc review | Fix the 24 findings in §4. Tool descriptions open with the purpose, and the access rule moves to its own field. In omp the essential tools are `bus_send_direct`, `bus_send_topic`, `bus_wait`, `bus_read_thread` and `bus_add_post` (PO ruling via omp-parity); `bus_inbox_ack` is not offered to the model there. `bus_spawn_worker` gets an entry in `TOOLS.md`. | server registry + extension |
| 2 | Context on connect | **No one-time injection.** While the session has bus credentials, the extension's `context` event handler puts an identity block (name, role, space, leaders) plus the guide at the head of every model request. Subagents get subagent lines instead of the guide (blackboard tools only, posts show the main session's name). An admin-token session is a full agent session of that named admin (inbox included) and gets the admin variant: "acts as a leader in every space" plus its joined spaces. | extension |
| 3 | Leader/member usage | **One guide, served by the server.** It is embedded markdown of at most 4000 characters, served as MCP `instructions`, as `GET /v1/guide` for the extension, and as a section of `TOOLS.md`. It covers leaders, members, admin and omp subagents. The **skill is retired completely** (PO ruling; cutover in §6). | server (+ extension consumer) |
| 4 | First prompt from the bus | **Decided (round 3), option A:** the server stores the invite task as a `direct_message` from the inviter (a leader's or an admin's name), and `task` leaves the accept response. The worker gets it as an ordinary `agent-bus` direct-message entry from the inviter (envelope format owned by `docs/design/specs/omp-parity/omp-rendering.feature`). The extension stops calling `sendUserMessage`. The print-mode race is accepted risk R1, mitigated by a bounded 5 s wait in `session_start` (§5.4b). A new extension on an old server would lose the task; the server protocol check in `docs/design/specs/findings/plugin-version-skew.feature` prevents that pair. | server (invites → inbox) + extension |
| 5 | Space overview | **New tool `bus_space_overview {space}`** that returns you, participants, topics with your subscriptions, boards and the 10 most recent threads. The guide and the block tell the agent to call it first. Nothing is injected on join. | server |

Answer to the product owner: **a skill is the wrong primary carrier.** omp guides agents through its own tools by *pushing* text. Each tool has a long description. The system prompt has sections gated on active tools. Every incoming peer message comes in an envelope that says how to reply. Subagents get a roster block that tells them who they are, who their peers are and when to message (§3.1). omp uses skills for *optional* domain workflows that the agent chooses to load (§3.1 E7). The bus is the opposite case. Identity, role and permissions are per-session state, and every connected agent needs them on its first turn. A spawned worker today gets a plain user prompt that never mentions the bus, so it has no reason to load the skill (§4 D1).

## 2. Current-state map

| Item | What exists today | Where |
|---|---|---|
| 1 Docs | The skill has role tables, leader and member workflows, pitfalls and errors. Tool descriptions are one or two sentences, access rule first. `TOOLS.md` is generated from the registry, splits off the first sentence as "Access", and has no entry for `bus_spawn_worker`. | `skills/agent-message-bus/SKILL.md`; `server/internal/tools/{spaces,topics,blackboards,inbox,invites,admin}.go`; `server/internal/tools/doc.go:70-74` (`splitDescription`); `extension/lib/spawn-tool.ts:22-23`; `server/TOOLS.md` |
| 2 Context on connect | Nothing. Connecting registers tools and starts the inbox stream (`extension/lib/connection.ts:52-60`). The only reply is a `ctx.ui.notify` to the *user* (`extension/index.ts:155-156,176-177`). No `before_agent_start` or `context` handler (`extension/index.ts:162-190`). | extension |
| 3 Usage | Only the skill (pull-based). MCP clients get no `instructions`: `mcp.NewServer(&mcp.Implementation{…}, nil)` (`server/internal/mcpserver/server.go:21`). | skill, mcpserver |
| 4 First prompt | Invites store `task` (`server/internal/invites/service.go:53-60`, `repository.go:12-19`), but not the inviter. Accept returns `{name, space, token, task?}` (`service.go:30-37,107`). The extension sends `pi.sendUserMessage(joined.task)` (`extension/index.ts:101-105`; `extension/lib/accept.ts:31`). So the worker sees an unmarked user prompt and doesn't know who sent it. | invites, extension |
| 5 Overview | 4+ calls: `bus_whoami`, `bus_list_participants`, `bus_list_topics`, `bus_list_blackboards`, plus `bus_list_threads` per board (`SKILL.md:48`). No query for threads across boards. | spaces, messaging, blackboards |

Facts the new behaviour relies on:
- `inbox.Service.Deliver(ctx, fn)` runs `fn` in one transaction and publishes after commit (`server/internal/inbox/deliver.go:12`). Every other producer already uses it, e.g. `messaging/service.go:42-45` and `spaces/service.go:183-185`.
- `inbox.from_name` is TEXT (`server/migrations/0001_init.up.sql:31`), so an entry "from" a leader who has since left is valid.
- Departures create no entry for leaders, except `blackboard_removed` for thread subscribers (`server/spec/membership.feature:185-207`). A leader is not told when a member joins or leaves.
- Inbox delivery in omp today: `pi.sendMessage({customType:"agent-bus", content, display:true}, {deliverAs: critical ? "steer" : "followUp", triggerTurn: tui|rpc})` (`extension/lib/deliver.ts:17-25`). Round-2 ruling: normal entries become `aside`, and critical entries keep `steer`; the omp-parity BA owns that change.

## 3. Research findings

### 3.1 How omp guides agents (with evidence)

| # | Mechanism | Evidence | Relevance |
|---|---|---|---|
| E1 | **Long markdown tool descriptions**, one prompt file per built-in tool and rendered with templates. Example: `task.md` lists the available agents, and parts appear only when messaging is on (`{{#if ircEnabled}}`). | `CA/src/prompts/tools/*.md` (~95 files); `CA/src/tools/read.ts:48`, `CA/src/task/index.ts:32,196`; `CA/src/prompts/tools/task.md:15,26` | Tool descriptions are omp's main guidance channel. Ours are terse. |
| E2 | **Extension tools are "discoverable" by default.** Their schemas leave the top-level list and are mounted under `xd://` or found through tool search, so the model first sees a summary. | `CA/src/extensibility/extensions/types.ts:898-899`; `AC/src/types.ts:1046-1054,1145-1148` | All 30 `bus_*` tools are registered without `loadMode` (`extension/lib/tools.ts:30-43`, `spawn-tool.ts:37-48`). Whether they actually leave the top level depends on the session's discovery settings [INFERENCE]. If they do, the opening words of each description are what the agent sees first, and ours open with "Participants only." |
| E3 | `ToolDefinition` has **no** `promptSnippet`, `promptGuidelines` or `systemPrompt` field. `ToolInfo.promptGuidelines` exists but is never populated. | `CA/src/extensibility/extensions/types.ts:884-964,986-994`; `CA/src/session/session-tools.ts:790-807` | An extension can't attach system-prompt text to a tool. |
| E4 | **`before_agent_start`** can return `systemPrompt: string[]` (a complete replacement, chained across extensions) and `message`. | `CA/src/extensibility/extensions/types.ts:1064-1073,1562-1566`; `CA/src/extensibility/extensions/runner.ts:2233-2293`; `CA/src/session/agent-session.ts:8283,8336-8344` | The obvious system-prompt seam, **but** see E5. |
| E5 | `before_agent_start` runs **only for user-authored prompts or queued user messages**. A turn started by `sendMessage(…, {triggerTurn:true})` goes through `#promptAgentInitiatedMessage → agent.prompt` with no preparation. The override then lasts "until the next preparation". | `CA/src/session/agent-session.ts:8176-8206` (`isUserAuthoredQueuedMessage`), `:9767-9773`, `:9486-9511`; `CA/src/session/session-tools.ts:538-570` | A worker driven only by `[agent-bus]` messages, which is what item 4 proposes, **never** gets a `before_agent_start` system prompt. |
| E6 | **`context` event**: "Fired before each LLM call". The handler gets a deep copy of the messages and may return replacements. The session is not modified. | `CA/src/extensibility/shared-events.ts:184-196`; result `CA/src/extensibility/extensions/types.ts:1515-1517`; wired `CA/src/sdk.ts:4360-4363`; called per request `AC/src/agent-loop.ts:1917-1919,1941-1942` | Works on every request, including bus-triggered turns and subagents. It is the recommended seam for item 2. |
| E7 | **Skills** appear as name and description only, under "Matching skill → MUST read `skill://<name>` first". They are listed only if an active tool can read skill URIs. A plugin ships them as a sibling `skills/` folder. | `CA/src/prompts/system/system-prompt.md:42-50`; `CA/src/system-prompt.ts:924-939`; `CA/src/discovery/omp-plugins.ts:1-18`; `CA/src/discovery/omp-extension-roots.ts:344-385` | Pull-based, for optional workflows. The bus skill costs about 150 tokens of description (`SKILL.md:3`) in **every** omp session that has the plugin, even unconnected ones [INFERENCE: ~4 chars/token]. |
| E8 | **Rules**: `alwaysApply` rules are pasted into `<generic-rules>` on every request, and plugins ship them as sibling `rules/`. | `CA/src/prompts/system/system-prompt.md:51-64`; `CA/src/discovery/omp-plugins.ts:114-129`; `CA/src/discovery/builtin.ts:405-455` | Static text, and it can't be conditioned on bus credentials. It would load into every session with the plugin. Not suitable. |
| E9 | **Custom messages reach the model as `developer` role.** `customType` and `details` are not sent, so all framing must be in `content`. | `AC/src/compaction/messages.ts:214-227`; `CA/src/session/messages.ts:1124-1140,1221` | `[agent-bus]` entries and a context block are developer messages. Our header is the only framing the model sees. |
| E10 | **omp's own agent messaging (IRC).** An incoming peer message is wrapped in `<irc>…</irc>` with *reply instructions inside every message*. Agent-written bodies are escaped so a body can't forge an envelope. A message from the parent comes as a **user-role** steer `<irc from="parent" agent="…">`. | `CA/src/prompts/system/irc-incoming.md:1-9`; `CA/src/session/irc-bridge.ts:201-247`; `CA/src/prompts/steering/parent-irc.md` | A pattern to copy. See D21 (no escaping in `[agent-bus]`) and the omp-parity overlap. |
| E11 | **Roster/identity block** in the subagent system prompt: "Your id is …", the live peers, and when to message. "Your final result reaches Main automatically. Message Main only for questions, blockers, or decisions — never progress or completion reports." | `CA/src/prompts/system/subagent-system-prompt.md:27-49`; data `CA/src/task/executor.ts:358-390` | This is omp's own answer to "who am I, who are my peers, when do I report", and the model for the item 2/3 block. Unlike omp subagents, a bus member's result does **not** reach the leader automatically, so the bus guide *must* ask for completion reports. |
| E12 | **MCP server instructions**: omp appends them under "## MCP Server Instructions" and truncates each server's text at **4000 characters**. Users can turn them off with `instructions: false`. | `CA/src/sdk.ts:1408,4075-4086`; `CA/src/mcp/manager.ts:2022-2037`; `CA/src/mcp/types.ts:90-95` | Puts a 4000-character budget on the guide. |
| E13 | `tool_call`/`tool_result` handlers can return `additionalContext`, trusted text for the next request (it also covers failed calls). | `CA/src/extensibility/shared-events.ts:331-421` | Possible seam for error-specific guidance, e.g. after `forbidden`. Not needed now. |

### 3.2 How MCP-only clients get guidance

- The MCP schema defines `InitializeResult.instructions` as "Instructions describing how to use the server and its features … like a 'hint' to the model" (https://modelcontextprotocol.io/specification/2025-11-25/schema, `#initializeresult-instructions`; same in 2025-06-18).
- go-sdk v1.8.0 has `ServerOptions.Instructions` (`SDK/mcp/server.go:74-75`) and returns it both in `InitializeResult` (`:2100-2107`) and in the 2026-07-28 `DiscoverResult` (`:953-957`). So one option covers the old and the new protocol, and the stateless `/mcp` (`server/spec/README.md:84`) is no obstacle.
- **Today the bus sets none** (`server/internal/mcpserver/server.go:21`). MCP clients see only the 29 terse tool descriptions. The README sends humans to the SSE endpoint (`README.md:68`).
- Prompts (`AddPrompt`, `SDK/mcp/server.go:278`) are user-invoked templates in most clients, and resources must be attached or subscribed by the client [INFERENCE: client UX, not verified per client]. Neither reliably reaches the model on its own, so instructions plus tool descriptions are the channels that matter. MCP resource subscriptions for *mail* belong to the mail BA.
- Per-token instructions would be possible, because the handler builds the server per request (`mcpserver/server.go:25`, `func(*http.Request) *mcp.Server`) [INFERENCE: called per request in stateless mode]. Not recommended: it costs a server per request, and identity is one `bus_whoami` call away.

## 4. Doc review findings (gaps and contradictions)

Severity: **H** = an agent will act wrongly, **M** = it's confused or wastes calls, **L** = a cosmetic inconsistency.

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| D1 | H | **A spawned worker never learns it's on the bus.** Its first prompt is a plain user message with only the `task` text. Nothing loads the skill or says who the leader is. | `extension/index.ts:103`; `extension/README.md:27-28`; `SKILL.md:40` ("gets `task` as its first prompt") |
| D2 | H | **Report to which leader, and when?** "Report to your leader" (`SKILL.md:58`), yet "A space can have several leaders" (`SKILL.md:39`), and the worker isn't told who invited it. No rule says when to report (done, blocked, decision), and a member's result never reaches the leader automatically, unlike omp subagents (E11). | `SKILL.md:39,58` |
| D3 | H | **Who ends a membership?** The member workflow says "Leave … when done" (`SKILL.md:59`). The leader workflow says "Wind down: `bus_revoke_member`" (`SKILL.md:44`). A member that leaves deletes its boards and inbox, and the leader is **not** notified. The leader's next direct message then fails with `not_found`. | `SKILL.md:44,59`; `server/spec/membership.feature:130-135,185-207` |
| D4 | H | **The hand-off target is undefined.** "post it to a leader's board" (`SKILL.md:59`), but no convention says that a leader creates a results board or what it's called. | `SKILL.md:59` |
| D5 | M | **Topic discovery.** Creating a topic notifies nobody, and members "subscribe themselves" (`SKILL.md:42`) to topics they have no way of learning about. The `bus_create_topic` description doesn't say so. | `SKILL.md:42`; `server/internal/tools/topics.go:57` |
| D6 | M | **The inbox tools mislead omp agents.** `bus_inbox_read`/`bus_inbox_ack` are active in top-level omp sessions, but their descriptions don't say that entries are streamed and acked automatically. An agent may poll, or try to "ack" what it already got. *Partly resolved by the omp-parity ruling:* `bus_inbox_ack` is not offered to the model in omp; the `bus_inbox_read` description still needs the note. | `server/internal/tools/inbox.go:21,27`; `extension/lib/tools.ts:16-17` (excluded only for subagents) |
| D7 | M | **`critical` claims an interrupt everywhere.** "mark the message critical so it interrupts the recipient". Only omp recipients in tui/rpc are interrupted; MCP pollers just see `critical: true`. | `server/internal/tools/schema.go:51-53`; `extension/lib/deliver.ts:17-19` |
| D8 | M | **The descriptions lead with the access rule, not the purpose.** All 29 open with their access rule, and "Participants only" alone opens 16 of them. In omp's discoverable mode the start of the description is what the agent sees first (E2). `doc.go` depends on this order for the "Access" column. | `server/internal/tools/doc.go:70-74`; `spaces.go:27-64`, `topics.go:35-60`, `blackboards.go:39-82`, `inbox.go:21,27` |
| D9 | M | **`TOOLS.md` lacks `bus_spawn_worker`.** The "full per-tool reference" the skill links to (`SKILL.md:35`) documents only `/bus spawn` (`TOOLS.md:522`). The extension README documents the tool (`extension/README.md:78`). | `server/TOOLS.md:513-534`; `SKILL.md:35` |
| D10 | M | **The skill's reference link points to GitHub `main`.** It may not match the running server and is unreachable offline. MCP clients never see it. | `SKILL.md:35` |
| D11 | M | **Subagent replies go to the parent.** The skill says subagents have no inbox (`SKILL.md:8`) but not that a subagent's messages go out as the parent's identity, so replies land in the parent's inbox and a subagent can't wait for one. *Resolved by the round-2 ruling:* subagents get only the blackboard read/post tools and go through the main session for everything else (T8, `session-context.feature`). | `SKILL.md:8`; `extension/README.md:66,76` |
| D12 | M | **The member orientation takes 4+ calls** (whoami, participants, topics, blackboards, then threads per board), and nothing shows threads across boards. | `SKILL.md:48`; no cross-board query in `server/internal/blackboards/threads.go:42-53` |
| D13 | M | **The `bus_create_blackboard` description omits the key pitfall**: a member's boards die when it leaves or is revoked. That is the reason for the hand-off rule. | `server/internal/tools/blackboards.go:44`; `SKILL.md:64` |
| D14 | M | **No guide for MCP clients.** They get no `instructions` and no workflow guidance at all. | `server/internal/mcpserver/server.go:21` |
| D15 | M | **`bus_spawn_worker`'s `task` doesn't say what a good task contains** (goal, done criteria, where to report) or its length limit. The server's `task` property states `1..65536 bytes`. | `extension/lib/spawn-tool.ts:30`; `server/TOOLS.md:110` |
| D16 | L | **The admin's abilities conflict.** The skill says admin "Only `bus_mint_leader`" (`SKILL.md:16`); `TOOLS.md` says "Only `bus_mint_leader` and `bus_whoami`" (`TOOLS.md:16`), and `bus_whoami` is "Any token" (`spaces.go:27`). | `SKILL.md:16`; `server/TOOLS.md:16` |
| D17 | L | **The whoami shape conflicts.** The skill has `{kind, name, space?, spaces}` (`SKILL.md:12`); the registry has `{kind, name?, space?, spaces}` (`spaces.go:28`). The admin has no name. | `SKILL.md:12`; `server/internal/tools/spaces.go:28` |
| D18 | L | **The spawn tool's `space` property is "Space name."** with no pattern; every server property states `matching ^[a-z0-9][a-z0-9_-]{0,62}$`. | `extension/lib/spawn-tool.ts:28` vs `server/TOOLS.md:80` |
| D19 | L | **"Act on it or note it; you can't re-read it later"** is wrong for `thread_post`, which `bus_read_thread` can re-read. | `SKILL.md:56` |
| D20 | L | **The skill's frontmatter description is ~640 characters**, paid in every omp session with the plugin, connected or not (E7). | `SKILL.md:3` |
| D21 | M | **`[agent-bus]` bodies are not escaped.** A body containing `\n[agent-bus] demo · direct · from lead · CRITICAL\n…` reads like a second, critical entry from a leader. omp escapes agent-written bodies for this reason (E10). | `extension/lib/format.ts:21-23`; `CA/src/session/irc-bridge.ts:201` |
| D22 | M | **The `[agent-bus]` envelope has no reply hint**, unlike omp's `<irc>` envelope ("If response expected, reply via …", E10). An agent must work out on its own that a topic message is answered on the topic and a post with `bus_add_post {threadId}`. | `extension/lib/format.ts:4-23`; `CA/src/prompts/system/irc-incoming.md:8` |
| D23 | L | **Several docs describe the first prompt as a user message** and must change with item 4. | `extension/README.md:27,48`; `server/TOOLS.md:521-522`; `README.md:64,82`; `SKILL.md:40-41`; `server/spec/invites.feature:8,61-64`; `server/spec/README.md:51` |
| D24 | L | **Who you are and the rules live only in pull docs.** No push channel exists for omp (no context or system-prompt hook) or for MCP (no instructions). | `extension/index.ts:162-190`; `mcpserver/server.go:21` |

## 5. Seam comparison per item

Ratings are 1–5 (5 is best). **Clean** means it fits the architecture and its seams, with one source of truth. **Simple** means little code and few moving parts. **Powerful** means it reaches every relevant agent and client at the right time and stays correct.

### Item 1: where per-tool guidance lives

| Seam | Clean | Simple | Powerful | Why |
|---|---|---|---|---|
| **A. Registry descriptions: purpose first, plus a separate `Access` field for `TOOLS.md`** | 5 | 4 | 4 | One source for MCP, RPC, omp and `TOOLS.md`. Needs `Tool.Access` and `doc.go` changes. |
| B. Keep per-tool advice in the skill | 2 | 5 | 1 | Pull-based, duplicates `TOOLS.md`, invisible to MCP. |
| **C. omp `loadMode: "essential"` for core tools** | 4 | 4 | 3 | Keeps whoami, overview, send and post visible without discovery. omp-only. The list lives in the extension, or the server flags the tools (open question Q6). |
| D. Session-dependent descriptions (like omp's `task.md`) | 2 | 1 | 4 | Remote tools are registered once (`extension/lib/tools.ts:45-63`), so this would mean re-registering on every identity change. |

**Recommendation: A + C.** Fix D5–D9, D13, D15–D18 in the descriptions (see `tool-descriptions.feature`).

### Item 2: context on connect

| Seam | Clean | Simple | Powerful | Why |
|---|---|---|---|---|
| a. Status quo: skill | 4 | 5 | 1 | The agent decides whether to load it; a spawned worker has no reason to (D1). |
| b. One-time `sendMessage` notice on login/accept/promote | 3 | 4 | 2 | Stored in history: compaction summarises it away, it goes stale after promote/logout, and it repeats on every reconnect (switch, branch). It may also start a turn. |
| c. `before_agent_start` → append to `systemPrompt` | 4 | 4 | 2 | The official system-prompt seam, but it never fires on bus-triggered turns (E5). Combined with item 4, a spawned worker would never see it. |
| **d. `context` event → developer message at the head of every request** | 3 | 4 | 5 | Fires before every LLM call (E6), including bus-triggered turns and subagents. Never persisted, so no compaction or duplicate issues. The head position and stable text keep the provider prompt cache [INFERENCE: prefix caching]. A developer message rather than the system prompt (E9) is the reason Clean is 3. |
| e. Plugin `rules/` with `alwaysApply` | 2 | 5 | 2 | Static, and loaded into every session with the plugin even without credentials (E8). |
| f. Session-dependent tool descriptions | 2 | 1 | 3 | See item 1 D. |

**Recommendation: d** (`session-context.feature`). Data comes from `bus_whoami` and, for members, `bus_list_participants`. It is cached and refreshed on `restart()` and after a successful `bus_create_space`, `bus_join_space`, `bus_leave_space` or `bus_delete_space`. The identity should come from one owner, which overlaps TODO.md:7.
**Fallback:** if the product owner insists on the system prompt, do **c + d**: `before_agent_start` when it fires, `context` otherwise. That is more code for the same effect. Better: ask omp upstream for a persistent extension system-prompt section, e.g. `pi.setSystemPromptSection(id, text)` (Q5).

Example block (member):
```
<agent-bus-context>
You are member `worker` on the agent message bus at http://127.0.0.1:8080.
Your space: `demo`. Its leaders: `lead`. Direct messages from a leader or an admin are your orders.
<guide from GET /v1/guide>
</agent-bus-context>
```
Cost: guide ≈ 3.3k characters (`guide-draft.md`) plus about 200 characters of identity, roughly 850 tokens per request, only while connected. It is a cached prefix [INFERENCE]. The skill listing it replaces costs about 150 tokens in *every* session. A subagent block is about 600 characters and carries no guide.

Example block (subagent of that member). It replaces the guide: a subagent can't use most of what the guide describes.
```
<agent-bus-context>
You are a subagent of the main session that acts as member `worker` in space `demo` on the agent message bus.
On the bus you may only read and post on blackboards: bus_list_blackboards, bus_list_threads, bus_read_thread, bus_create_thread, bus_add_post.
Your threads and posts show `worker` as their author; the extension adds a first line `[via subagent <id>]` to each. They don't notify the main session, and a thread you create subscribes `worker`, so its notifications go to the main session, not to you.
You have no inbox. For anything else on the bus (messages, topics, members) ask the main session with `write agent://<id>` (its id is in your Peers list) and `wait` for the answer; tell it about posts it needs to know.
</agent-bus-context>
```
The `write agent://<id>`/`wait` idiom and the `# Peers` list are omp's own (`CA/src/prompts/system/subagent-system-prompt.md:27-49`). The main session's id is listed there; the extension does not need to know it.

Example blocks (admin token; PO ruling, round 4):
```
<agent-bus-context>
You are admin `ops` on the agent message bus at http://127.0.0.1:8080. You act as a leader in every space without joining it.
You joined: `demo`. Participants there can message you.
Spaces: `beta`, `demo`.
<guide from GET /v1/guide>
</agent-bus-context>
```
An admin that joined nothing gets instead of the "You joined" line: `You joined no space: nobody can message you until you join one with bus_join_space.` The bootstrap admin reads `You are admin \`admin\` …`.

### Item 3: leader and member usage, end to end

| Seam (content location) | Clean | Simple | Powerful | Why |
|---|---|---|---|---|
| a. Expand the skill | 3 | 5 | 1 | Pull; omp only (plugin `skills/`); a second copy for MCP would drift. |
| **b. Server-embedded guide → MCP `instructions` + `GET /v1/guide` + `TOOLS.md` section** | 5 | 3 | 5 | One source, versioned with the server that enforces the rules. Reaches omp (via the block) and every MCP client. Needs a new route, an mcpserver option and a tooldoc template. |
| c. Guide text embedded in the extension (TS constant) | 3 | 4 | 3 | omp only. MCP clients need a second copy. |
| d. Tool descriptions only | 4 | 4 | 2 | Can't express workflows across tools (spawn → report → hand-off → revoke). |
| e. MCP prompts or resources | 3 | 3 | 1 | User-invoked or attached by the client [INFERENCE]; doesn't reach the model on its own. |

**Recommendation: b + d.** The draft is in `guide-draft.md` (3854 characters, under omp's 4000 cap, E12), and the content requirements are in `agent-guide.feature`. End-to-end flows covered:
- **Leader:** create the space, a topic per workstream and a results board. Spawn or invite workers with a name and a task (goal, done criteria, where to report). Answer questions; use `all` only for announcements and `critical` only to stop or redirect. Read the result, revoke the worker, delete the space when the work is done.
- **Member:** the first direct message from a leader or an admin is the task, and every such message is an order. Orient with `bus_space_overview`. Report to the task's sender when done, blocked or needing a decision, not on progress; if that fails with `not_found` (an admin that has not joined), report to a leader of the space. Put lasting results in a thread. Leave only when told, and hand off to a leader's board first.
- **Permissions:** members can't send to `all` or send critical. Topics, boards and threads are open to every participant. Removal is limited to the owner or creator, or a leader. Admins are a strict superset of the leader role in every space without joining: they may also delete any space and revoke members and leaders. They are reachable only where they joined.
- **Mail (PO ruling):** only critical entries steer, interrupting the running turn. Normal entries are queued as omp `aside`s: they arrive at the next step without stopping the work, or start a turn when the agent is idle. Delivery mechanics belong to the omp-parity BA, and the non-omp lines ("You have mail" hint, `bus_wait`) to the mail BA; the guide states only the effect ("acknowledged once you have seen them"). In omp, `bus_inbox_ack` is not offered to the model.
- **omp subagents (PO ruling):** only the five blackboard read/post tools. Threads and posts show the main session's bus name, don't notify the main session, and a created thread subscribes the main session. Everything else goes through the main session via `write agent://<id>` and `wait`.

### Item 4: first prompt recognisable as coming from the bus

| Seam | Clean | Simple | Powerful | Why |
|---|---|---|---|---|
| a. Status quo `sendUserMessage(task)` | 3 | 5 | 1 | Unmarked, and the sender is unknown. |
| b. Option B: the accept response keeps `task` and adds `from`; the extension renders it as an `[agent-bus] … direct · from <leader>` message and delivers it through the same policy as stream entries; no inbox entry (**rejected**, round 3) | 3 | 4 | 3 | Same look as other entries and a small change. But it is a second delivery path next to the inbox, it is lost if the session fails before delivering it, and MCP agents never see it (§5.4b). |
| **c. Option A:** at accept, the server stores the task as a `direct_message` entry from the inviter in the same transaction (`inbox.Deliver`); `task` leaves the accept response. The extension sends nothing. | 5 | 3 | 5 | It is *literally* a direct message from that leader: same format, same ack, durable until acked, and identical for omp and MCP clients. Needs a migration (`invites.invited_by`) and the invites service routing Accept through `inbox.Deliver`. Removes code from the extension. |
| d. The leader's extension sends `bus_send_direct` once the worker shows up | 2 | 1 | 3 | Racy; it has to poll participants. |
| e. Pre-create the member at invite time and queue the message then | 1 | 2 | 3 | Breaks "no agent worker exists yet" (`server/spec/invites.feature:22`) and the revoke semantics. |

**Decided (round 3): option A (c).** The comparison and the accepted print-mode risk are in §5.4b. The worker identifies the order through the block ("Direct messages from a leader or an admin are your orders") and the `from <inviter>` header, and knows from the same header whom to report to; that closes D1 and D2. If the report fails with `not_found` (an admin that has not joined the space), the worker reports to a leader of the space. The round-1 user-role variant was dropped: under the PO's delivery ruling every normal entry is an `aside`, so a user message would be the one entry that looks different.

### 5.4b Invite task: option A (decided, round 3)

The PO chose **option A**: the server stores the task as a `direct_message` from the inviter when the code is accepted, and `task` is removed from `POST /v1/invites/accept`. The comparison that led there:

| Aspect | A (chosen): server-stored direct message | B (rejected): `task` + `from` in the accept response |
|---|---|---|
| **Durability** | Stored in the inbox until it reached a session's context (omp-parity ack rule, `docs/design/specs/omp-parity/omp-delivery.feature`). It survives a crash between accept and the first turn, and a second session that holds the stream slot. | Only in the HTTP response and then in the session. Lost if the extension fails between accept and delivery (`extension/index.ts:101-105`). |
| **Duplicate delivery** | At least once, like every entry: a crash after the entry reached the context but before the ack replays it. Rare. | Exactly once per accept. |
| **MCP clients** | The agent finds its task, with its sender, at its first `bus_inbox_read`/`bus_wait`. Codes are redeemed with `curl`, outside MCP (`server/spec/invites.feature:51-52`). | The task sits in the human's `curl` output. |
| **Plain HTTP clients** | Breaking: they read the task from the inbox (`README.md:82` changes). | Compatible. |
| **omp worker in tui/rpc** | The entry arrives as an `aside` and starts a turn when idle. | Same. |
| **Print mode** | A race; see the accepted risk below. | Deterministic. |
| **One delivery path** | Yes. | No: an omp-only second path. |
| **Effort** | Server medium (migration, inbox dependency in `invites`, `Admission.Task` removed); extension negative. | Server small; extension small. |

**Accepted risk R1: the print-mode race.** With `omp -p <prompt> --bus-url … --bus-accept …`, the task entry arrives on the inbox stream asynchronously after the accept. The one-shot run does not wait for it:
- if it arrives before the run starts, it is added to the context and acted on;
- if it arrives during the run, it is injected at the next step boundary and acted on only if another model call follows;
- if the run ends first, this process never acts on it. The entry stays unacknowledged in the inbox (omp-parity ack rule) and goes to the next session of that identity.

Spawned workers are not affected, because every launcher starts an interactive session (`extension/README.md:84-94`). The risk only touches scripted one-shot workers.

**Mitigation, in scope (small, recommended): a bounded wait in `session_start`.** omp awaits the `session_start` handlers before a print/json run starts (`CA/src/modes/runtime-init.ts:212`; `CA/src/modes/print-mode.ts:167,434`). It works like this:
- After redeeming `--bus-accept` in print or json mode, the extension calls `bus_inbox_read` once (it does not acknowledge). The task entry is committed before the accept response returns, so it is already visible.
- It then lets `session_start` finish only when the stream has added every entry up to the highest id it read to the context, or after 5 seconds. On timeout it logs `agent-bus: inbox backlog not delivered within 5 s; continuing`.
- No server change.
- Interactive modes and sessions without `--bus-accept` don't wait.
- A general "deliver the backlog before any one-shot run" rule would belong to the omp-parity BA.

Specified in `first-prompt.feature`, rule "A one-shot worker waits briefly for its task". What remains of R1 after the mitigation is a stream that doesn't deliver within 5 s; the entry then follows the normal print-mode delivery.

### Item 5: overview for a newly joined agent

| Seam | Clean | Simple | Powerful | Why |
|---|---|---|---|---|
| **a. Server tool `bus_space_overview {space}`** (read-only package `internal/overview` composing spaces, messaging and blackboards reads; one new query for recent threads across boards) | 4 | 3 | 5 | All clients, always current, one call. Owns no table. |
| b. The extension composes the existing list tools and injects a snapshot on join | 2 | 3 | 2 | omp only, N+3 calls, and a stale snapshot is left in history. |
| c. Extend `bus_whoami` with space content | 2 | 4 | 3 | Mixes identity with space state. A leader has many spaces. |
| d. Put the overview into the per-request block | 3 | 2 | 3 | A server call per LLM request, and the cache breaks on every new post. |
| e. MCP resource `bus://spaces/{s}/overview` | 3 | 2 | 2 | Clients rarely let the model read resources unprompted [INFERENCE]. MCP only. |

**Recommendation: a**, and the guide and block say "call `bus_space_overview` first" (`space-overview.feature`). The result shape is fixed in the feature's header comment.

## 6. Skill cutover (PO ruling: retired completely)

No skill remains, not even a connect-only one. Nothing replaces the skill listing for **unconnected** sessions. The user learns how to connect from `/bus help` and the `/bus` command replies (`extension/README.md:22-35`), and an agent can't run `/bus` commands anyway.

**What replaces each part of `skills/agent-message-bus/SKILL.md`**

| SKILL.md part | Lines | Replaced by |
|---|---|---|
| Frontmatter `description` (when to use; credential commands) | 3 | Nothing for unconnected sessions; `/bus help` for users. Connected agents get the context block. |
| Intro: spaces, inbox, tools active only with agent credentials, how to get credentials, logout, subagents | 8 | Guide intro and "Mail". Credential commands go to `extension/README.md` (humans) and the guide's `unauthenticated` advice ("tell the user"). Subagents get the subagent block and the guide's "omp subagents". |
| Who am I (whoami, kinds, role per space, admin row) | 10-20 | Block identity lines (kind, name, role, space, leaders); the `bus_whoami` description; guide "Admin" and the admin block. |
| Tools table | 22-31 | The registered tools themselves and `TOOLS.md`. |
| Name, body and title limits | 33 | Guide "Rules"; schema property descriptions (`server/internal/tools/schema.go`). |
| Link to `TOOLS.md` on GitHub | 35 | Dropped; `TOOLS.md` embeds the guide for humans. |
| Leader 1: create or join, several leaders | 39 | Guide "Leaders"; `bus_create_space`/`bus_join_space` descriptions. |
| Leader 2: `bus_spawn_worker` (launchers, cwd, name conflict, no launcher) | 40 | `bus_spawn_worker` description (`tool-descriptions.feature`); guide "Leaders". |
| Leader 3: invite by hand (code TTL, `/bus accept`, `--bus-accept`, `POST /v1/invites/accept`, url from `/bus status`, re-invite) | 41 | `bus_invite_member` description; `extension/README.md`. |
| Leader 4: topics, `all` | 42 | Guide "Leaders"; `bus_create_topic`/`bus_send_topic` descriptions. |
| Leader 5: critical | 43 | Guide "Mail" and "Leaders"; `critical` property. |
| Leader 6: wind down (revoke, delete, last leader) | 44 | Guide "Leaders"; `bus_revoke_member`, `bus_delete_space`, `bus_leave_space` descriptions. |
| Member 1: orient | 48 | Guide "Start"; `bus_space_overview`. |
| Member 2: inbox format, ack on delivery, CRITICAL | 49-56 | Guide "Mail". |
| Member 3: polling without the stream | 57 | Guide "Mail"; `bus_inbox_read`/`bus_inbox_ack` descriptions. |
| Member 4: report | 58 | Guide "Members". |
| Member 5: leave and hand-off | 59 | Guide "Members"; `bus_leave_space` description. |
| Blackboards and threads | 61-66 | Guide "Members" (lasting results in threads) and "omp subagents"; descriptions of `bus_create_blackboard` (owner; deleted with a member), `bus_create_thread` (subscribes), `bus_add_post` (notifies all but the author), `bus_list_threads` (order), `bus_remove_thread` (permission). |
| Pitfall: one identity per agent, promote per session | 70 | `bus_spawn_worker` description ("own name"); the extension's existing warning log; `extension/README.md` (promote is the user's concern). |
| Pitfall: `all` can't be subscribed or closed; closing frees the name | 71 | `bus_subscribe_topic`, `bus_unsubscribe_topic`, `bus_close_topic` descriptions. |
| Pitfall: no self-notification; no DM to yourself | 72 | `bus_send_topic` and `bus_add_post` (already "except the sender/author"); `bus_send_direct` description. |
| Pitfall: member token is valid in one space | 73 | Block "Your space: …"; guide errors line. |
| Errors table | 75-85 | Guide "Rules" (one line per code); the `TOOLS.md` "Errors" section (exists). |
| Secrets | 87 | Guide "Rules". |

The description additions this needs beyond round 1 are added to `tool-descriptions.feature`: `bus_invite_member`, `bus_subscribe_topic`, `bus_close_topic`, `bus_send_direct` and `bus_join_space`.

**Files that reference the skill** (grep over the repo, round 2; nothing in `tests/`, `scripts/`, `.github/` or server code):

| File | Lines | Change |
|---|---|---|
| `skills/agent-message-bus/SKILL.md` | all | Delete, along with the then-empty `skills/`. |
| `package.json` | 30 | Remove `"skills"` from `files`. The `omp` manifest (`package.json:23-27`) has no skills key: omp finds plugin skills through the sibling `skills/` folder (`CA/src/discovery/omp-plugins.ts:1-18`, `CA/src/discovery/omp-extension-roots.ts:344-385`), so deleting the folder unregisters the skill. |
| `README.md` | 11, 17, 93, 101 | The intro sentence says "a skill that tells agents how to use the bus": replace with "the server's agent guide, injected into every connected session". Remove the mermaid node `skill`. Replace the docs-table row and the layout row with the guide (`server/internal/guide/`). |
| `extension/README.md` | 9, 15 | "Agent-facing usage guidance is in SKILL.md" becomes the guide and context block. "ships this extension and the skill" becomes "ships this extension". Add a section on the context block. |
| `server/README.md` | 11 | "Agent-facing usage is in skills/…" becomes the guide (`internal/guide`, `GET /v1/guide`, MCP instructions). |
| `server/DEPLOYMENT.md` | 9, 155, 201 | Remove the "Skill" component row; "Install the plugin (extension and skill)" becomes "Install the plugin"; replace the skill reference with the guide. |
| `.omp/AGENTS.md` | 7, 17, 84 | Drop `skills/` from the stack line and layout table. The review checklist line "extension README and skill still accurate" becomes "extension README and the guide (`server/internal/guide/guide.md`) still accurate". |
| Installed plugins | n/a | An installed 0.1.0 keeps its skill until `omp plugin upgrade` [INFERENCE]. See the overlap with `docs/design/specs/findings/plugin-version-skew.feature`. |

## 7. Requirement trades (stated explicitly)

| ID | Asked for | Proposed instead | Why it's better | What's lost |
|---|---|---|---|---|
| T1 | Inject context **on connect** (login, accept, promote) | Context in **every model request while connected** | Survives compaction, promote, logout, session switch and branch. No duplicates. Always current. | Nothing visible in the transcript at connect time (the user notice stays). |
| T2 | Rethink the skill | **Decided (round 2):** the skill is retired completely; replaced by the server guide (MCP + omp block) and better tool descriptions (§6) | One source shared with MCP. Reaches spawned workers. Saves the skill listing in unconnected sessions. | On-demand deep reference. The errors table moves into the guide, and details stay in `TOOLS.md`. Unconnected agents lose the "ask the user to `/bus login`" hint. |
| T3 | First prompt "formatted like other `[agent-bus]` entries" | The first prompt **is** an `[agent-bus]` direct-message entry, stored by the server (option A, decided) | Durable, identical for every client, no special path. | It arrives as a developer-role message, not user-role (E9). omp's parent→child steer uses user role (E10). The block's "orders" line makes up for it. |
| T4 | (implied) keep `task` in the accept response | **Decided (round 3):** `task` is dropped from `POST /v1/invites/accept` | One delivery path; no double delivery. | Breaking for HTTP clients that read `task` (`README.md:82`). Print-mode race: accepted risk R1 with a bounded wait (§5.4b). |
| T5 | Overview "tool **or** block injected on join" | Tool only, plus a directive in the guide and block | Always fresh, works for MCP, no stale snapshot. | The agent spends one tool call. |
| T6 | "system-prompt addition" | A developer message at the head of the request (`context` event) | `before_agent_start` misses bus-triggered turns (E5). | Not literally the system prompt. Upstream ask Q5. |
| T7 | Worker knows "the leader that spawned it" | The first DM's `from` header and the block's list of leaders. No `invitedBy` in `bus_whoami`. | No new identity field. Covers the multi-leader case through the task's sender. | After compaction the inviter is known only if the summary kept it. With a single leader the block covers it. |
| T8 | (round 2) subagents use the bus | Subagents get only the five blackboard tools and reach the main session through omp's `write agent://<id>`/`wait` | One bus identity per agent is used by one session for messaging; replies can't go astray to a session that has no inbox (D11). | A subagent cannot message other agents or read topics directly; it costs a round trip through the main session. |

## 8. Target layering

1. **Tool descriptions** (server registry and `spawn-tool.ts`): what each tool does, who may use it, its pitfall. All clients.
2. **Guide** (server, embedded, at most 4000 characters): how to work as a leader or member. MCP `instructions`, `GET /v1/guide`, `TOOLS.md`.
3. **Session block** (omp extension, `context` event): who I am, my role, my space, my leaders, plus the guide. A subagent gets its subagent lines instead of the guide, and an admin-token session gets the admin variant (joined spaces or "joined no space"). Only while connected.
4. **Space state** (`bus_space_overview`): pulled on demand.
5. **Entry envelope** (`[agent-bus]` header): who sent this and where it came from. Possible reply hint and escaping (D21, D22) belong to the omp-parity and mail BAs.

### Change sketch (for planning; no code written)
- Server: migration `NNNN_invites_invited_by.up.sql` adding `invites.invited_by TEXT` (the inviter's name: a leader or an admin). It takes the next free number after `0001_schema.up.sql` at merge time and is never folded into it (`docs/design/specs/REVIEW.md` §11, slice 7). `invites.Service.Create` stores the caller's name. `Accept` runs in `inbox.Deliver` and emits the direct message; `Admission.Task` is removed. The server bumps its protocol version (`Agent-Bus-Protocol`, `docs/design/specs/findings/plugin-version-skew.feature`), and the extension of the same release requires it.
- Server, everything else:
  - New `internal/overview` package, `tools/overview.go`, and a recent-threads query in `blackboards/repository.go`.
  - New `internal/guide` (go:embed); `mcpserver.NewHandler` gets `ServerOptions{Instructions}`; a `GET /v1/guide` route in `internal/server`.
  - `Tool.Access` and `doc.go`; a new `ledger_*` template section.
  - Specs: `invites.feature`, `transports.feature` (`bus_space_overview` in the registry; guide route; instructions; the final registry after every proposal is 34 server tools, and `docs/design/specs/REVIEW.md` §11 gives the count at each merge step), new `overview.feature`. Regenerate `TOOLS.md`.
- Extension:
  - `index.ts` `join()` drops `sendUserMessage`, and `accept.ts` `Joined.task` goes.
  - The bounded wait: after `--bus-accept` in print/json mode, read `bus_inbox_read` once and hold `session_start` until the stream has queued every entry up to that id for the run's first model request (`nextTurn`, `docs/design/specs/omp-parity/omp-delivery.feature` one-shot rule), at most 5 s.
  - New `lib/bus-context.ts` (identity cache and rendering, plus the subagent and admin variants) and a `context` handler registration; a refresh hook after the membership tools.
  - Admin-token sessions: the admin token as the lowest-precedence active credential, giving a full agent session (tools, inbox stream, `bus_wait`).
  - The subagent tool filter: the five blackboard tools instead of today's "all except inbox" (`tools.ts:16-17`, `connection.ts:56`).
  - A `loadMode: "essential"` list; a rewrite of the `spawn-tool.ts` description.
- Docs: the skill cutover in §6.
- Suggested order: (1) description and doc fixes, (2) item 4 (option A, with the bounded wait), (3) item 5, (4) guide and MCP instructions, (5) extension block, subagent filter, admin-token sessions and skill retirement in one change. Step 5 depends on 3 and 4 and on the admin BA's server work; step 2 is independent.

## 9. Overlaps with the other BAs

- **Mail (`docs/design/specs/mail/`):**
  - The guide's "Mail" section adopts the mail BA's proposed lines (`docs/design/specs/mail/analysis.md` §7): the "You have mail" hint on tool results for non-omp clients, `bus_wait {timeoutSeconds, from?, threadId?}` as the one long-poll verb (their `bus-wait.feature`), and the PO ruling that only critical steers. They are shortened to fit the budget; see the drafting notes in `guide-draft.md`.
  - If the mail design changes again, the mail BA's text wins and the guide is updated to match.
- **omp parity (`docs/design/specs/omp-parity/`):**
  - It owns delivery (`aside`/`steer`, acknowledgement once an entry reached the context, print mode); the guide only describes the effect.
  - Option A's durability in print mode relies on its ack-on-injection rule (§5.4b).
  - Adopted from its round 2 (message from `BA-omp-parity`, `docs/design/specs/omp-parity/analysis.md:152,218,257-260,296,360`):
    - subagent posts get an automatic first line `[via subagent <id>]` (guide, block);
    - the five essential tools;
    - `bus_inbox_ack` is withheld from the model in omp;
    - the subagent allow-list is applied at registration in `ctx.agent.kind === "sub"`.
  - Also: envelope parity with `<irc>` (reply hint D22, body escaping D21), user-role steering for leader orders (T3), and the identity block as the bus counterpart of omp's `# Peers` roster (E11). Its S15 (subagents must act as the main session's identity in every login mode) is a precondition for the subagent block's "acts as `<name>`" line.
- **Postponed review findings (`docs/design/specs/findings/`):**
  - Item 4 touches the invite `task` validation; `TODO.md:9`'s wrong-field error ("body must be …", `invite-task-field.feature`) should be fixed with it.
  - Removing the skill ships in a new plugin version, so `plugin-version-skew.feature` applies: an installed 0.1.0 keeps the old skill until `omp plugin upgrade` [INFERENCE: upgrade replaces the package directory]. The same file now also has the extension refuse a server whose protocol is too old (item 4 needs the new server).
  - The identity cache for the block should have one owner (`TODO.md:7`, `SessionCredentials`).
- **Web frontend (`docs/design/specs/web/`) and admin (`docs/design/specs/admin/`):** `bus_space_overview`'s composite read is the natural backend for a space page. The admin BA owns the admin role (named agents, `admin` participant role, admin tools). The guide's "Admins" section, the admin block, the overview's admin cases and the inviter name follow its hand-off list (`docs/design/specs/admin/analysis.md` §11.1). The overview's participant order (admins, leaders, members) follows `admin-messaging.feature:80-89`.

## 10. Open questions for the product owner

1. **Q1 Skill:** *decided (round 2): retired completely, with no connect-only skill.* The cutover is in §6.
2. **Q2 Invite task delivery:** *decided (round 3): option A.* The server stores a direct message from the inviter, and `task` is removed from the accept response. The print-mode race is accepted risk R1, mitigated by a bounded wait (§5.4b).
3. **Q3 Ending membership:** should members leave only when a leader says so, with the leader revoking them (recommended and written into the guide)? Or should a member leave by itself after reporting?
4. **Q4 Join/leave notices:** should leaders get inbox entries when a member joins or leaves (new types such as `member_joined`/`member_left`)? This would fix D3's silent leave and confirm that a spawned worker is up. It's out of scope for these features.
5. **Q5 System prompt vs developer message:** accept the `context`-event block (recommended)? Or should we also file an upstream omp request for a persistent extension system-prompt section, since `before_agent_start` skips agent-initiated turns (E5)?
6. **Q6 Essential tools:** *decided (PO ruling to the omp-parity BA):* `bus_send_direct`, `bus_send_topic`, `bus_wait`, `bus_read_thread`, `bus_add_post`; the rest are discoverable. `bus_space_overview` therefore stays discoverable, and the guide's "Start" line points to it.
7. **Q7 Conventions:** do we fix a results-board convention (for example, the leader creates a board `results`, and members post their hand-off there)? Is reporting on done, blocked or decision (no progress reports) the right cadence?
8. **Q8 Overview contents:** should leaders also see pending invite names and members their unread count? Is a limit of 10 recent threads right?
9. **Q9 MCP instructions:** static for every token (recommended), or per token with the identity included?
10. **Q10 `GET /v1/guide` auth:** bearer like every `/v1` route (proposed), or public like `/healthz`?

Admin (PO ruling, round 4; supersedes the round-3 admin design). Server side: `docs/design/specs/admin/` (analysis §2–5, `admin-role.feature`, `admin-messaging.feature`, `admin-tokens.feature`):
- Admins are named agents of kind `admin`, each with its own inbox and stream slot.
- They have the admin tools (`bus_mint_leader`, `bus_mint_admin`, `bus_list_admins`, `bus_revoke_admin`) and leader rights in every space without joining.
- Participants can message an admin only in a space it joined, where its participant role is `admin`. Leaving keeps its authority.
- The bootstrap admin is `admin` (the instance token); more admins come from `bus_mint_admin`.

11. **Q11 Admin in omp:** *reopened and decided (round 4).* An admin-token session is a full agent session of that admin: bus tools active (admin tools included), inbox stream opened, `bus_inbox_read` and `bus_wait` registered, `bus_inbox_ack` not offered to the model (omp-parity ruling). The admin token has the lowest precedence: promote > env > accepted > stored agent token > admin token. This changes `/bus login <admin-token>` and `/bus server start`, which today activate nothing (`extension/README.md:26,32,57`). Several sessions with the bootstrap token collide on one stream slot (409, non-fatal); the admin BA recommends one minted admin per session or person (`docs/design/specs/admin/analysis.md` K3).
12. **Q12 Admin as sender:** *closed.* Entries and posts show the admin's name (`admin`, `ops`, …). A reply to an admin that has not joined fails with `not_found "participant ops not in space demo"` (`docs/design/specs/admin/admin-messaging.feature:46-49`). The guide tells members to report to the task's sender and, on `not_found`, to a leader of the space.
13. **Q13 Admin powers:** *closed (PO ruling, round 5; admin BA D10 and D11).* Admins are a strict superset of the leader role in every space: critical messages, `all`, inviting and spawning workers (`bus_spawn_worker`), deleting any space, and revoking members and leaders. The guide's "Admins" section states it.
14. **Q14 Admin and `bus_space_overview`:** *closed.* `you` is `{name, role: "admin"}` when the admin joined and `{name, role: null}` when not. Participants include the admin only when it joined, and are listed first (`space-overview.feature`).

## 11. Deliverables in this folder

| File | Content |
|---|---|
| `analysis.md` | This document. |
| `first-prompt.feature` | Item 4 (option A, decided): the invite task as a direct message from the inviter (a leader's or an admin's name), plus the bounded wait for one-shot workers. |
| `space-overview.feature` | Item 5: `bus_space_overview`. |
| `session-context.feature` | Items 2 and 3 (omp): the per-request bus context block, including admin-token sessions (full agent session, joined/unjoined variants) and subagents (blackboard tools only). |
| `agent-guide.feature` | Item 3 and the product owner's question: one server-served guide (MCP instructions, `GET /v1/guide`, `TOOLS.md`), with content requirements and no skill references. |
| `tool-descriptions.feature` | Item 1: description fixes, `bus_spawn_worker` in `TOOLS.md`, essential tools. |
| `guide-draft.md` | Draft guide text (3854 characters, under omp's 4000 cap). |
