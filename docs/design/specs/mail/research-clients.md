# How MCP clients handle server-to-client push (research as of 2026-10-26)

**Question:** can a Go MCP server (streamable HTTP, `modelcontextprotocol/go-sdk` v1.8.0) get mailbox messages to agents in real MCP clients without the agent polling a tool?

**How each push mechanism is graded.** There are three levels:

- **(a) received:** the client handles the message at the protocol level.
- **(b) model sees it:** the content is put into the model's context.
- **(c) idle wake:** a session sitting at the prompt starts a new turn with no user input.

**Sources.** Every claim below links its source and gives the version or date of that source where one exists.
- Docs were fetched on 2026-10-26.
- Source code was read at the commit named in each section, all dated 2026-10-23 to 2026-10-26.
- Anything not directly shown by a source is marked **[INFERENCE]**.

---

## 0. TL;DR

1. **Standard MCP gives you no way to push into the model.** No client checked takes the content of `resources/updated`, `list_changed` or `notifications/message` and gives it to the model, and none starts a turn because of them.
   - The best any client does is level (a): refresh its tool lists, write a log, or show progress to the user.
   - The spec doesn't require more. All of these are optional (MAY) features with "no mandated user interaction model" (§1).
2. **The newest spec, 2026-07-28, narrows push further.** It removes the standalone GET SSE stream and `resources/subscribe`. Push now happens only over the opt-in `subscriptions/listen` stream, and that stream carries nothing but list_changed and `resources/updated`.
   - go-sdk v1.8.0 negotiates 2026-07-28 by default ([release notes](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0): "`2026-07-28` remains the newest version the SDK negotiates").
3. **One mechanism works in every client: a long-poll tool, and only inside a turn.** The tool (e.g. `wait_for_mail`) blocks until mail arrives and returns it as its result, reaching level (b). Two things limit it:
   - It needs the model to call it, and it is bounded by each client's tool timeout. Only some clients reset that timeout when they receive progress notifications (§2, column 4).
   - It gives no level (c) by itself. The one exception is Claude Code, which moves long calls to the background and starts a turn when an MCP task completes (§3.6).
4. **Only Claude Code has documented ways for outside events to wake an idle session.**
   - **Channels** (`claude/channel`): a research preview, stdio only, opted in per session with `--channels` or a dev flag. Idle wake is disputed (issue #44380 is open).
   - **`asyncRewake` command hooks:** documented to wake an idle session on exit code 2.
   - **Monitor tool / plugin monitors:** stream a command's output lines into the session.
   - **MCP Tasks:** need protocol 2026-07-28 and the tasks extension, in public beta from v2.1.304. "If Claude is idle when the result arrives, Claude starts on it unprompted."
   - **Cross-session messaging inbox socket:** the documented path for a script or hook to post into a session.
5. **None of the other clients researched has an idle-wake path:** Codex CLI, Cursor, VS Code Copilot, Gemini CLI, Claude Desktop, Goose, Zed, Windsurf and Cline.
   - Several have end-of-turn hooks that can force one more turn: Cursor `stop.followup_message`, Codex `Stop` with block, Gemini `AfterAgent` with deny, Copilot `agentStop`.
   - Several have hooks that inject context during a turn (level b).
   - All of these are client-side configuration, not something a pure MCP server controls.
6. **Verdict on the PO claim:** largely **correct**; details in §6.

---

## 1. What the spec obliges clients to do (nothing, for push)

| Mechanism | 2025-06-18 / 2025-11-25 | 2026-07-28 (newest) |
|---|---|---|
| `resources/subscribe` + `notifications/resources/updated` | Optional on both sides: "Both `subscribe` and `listChanged` are optional", "the protocol itself does not mandate any specific user interaction model" ([resources 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18/server/resources)) | `resources/subscribe`/`unsubscribe` are replaced by `subscriptions/listen` with the opt-in type `resourceSubscriptions` ([changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)) |
| `*/list_changed` | Server "SHOULD send a notification" if it declared `listChanged`. The spec says nothing about what the client must do | Delivered only on `subscriptions/listen`, and only for types the client opted into: "The server MUST NOT send notification types the client has not explicitly requested" ([subscriptions](https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/subscriptions.md)) |
| `notifications/message` (logging) | Client "MAY" send `logging/setLevel`. No UI or model obligation ([logging](https://modelcontextprotocol.io/specification/2025-06-18/server/utilities/logging)) | Logging is deprecated (SEP-2577) and `logging/setLevel` is removed. Log messages are request-scoped: the server "MUST NOT emit `notifications/message` for requests that did not include" `io.modelcontextprotocol/logLevel` ([changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)) |
| Progress and timeouts | "Implementations **MAY** choose to reset the timeout clock when receiving a progress notification… **SHOULD** always enforce a maximum timeout" ([lifecycle#timeouts 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle#timeouts)) | Progress stays request-scoped: it is sent "on the response stream of the request they relate to" ([changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)) |
| Sampling / elicitation | Optional client capabilities. Sampling "SHOULD always be a human in the loop". Elicitation is aimed at the user ([sampling](https://modelcontextprotocol.io/specification/2025-11-25/client/sampling), [elicitation](https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation)) | Server-initiated requests are replaced by Multi Round-Trip Requests (`resultType:"input_required"`, SEP-2322). Sampling is deprecated (SEP-2577) |
| Standalone GET SSE stream | "The client **MAY** issue an HTTP GET… The client **MAY** close the SSE stream at any time". The server may answer 405 ([transports 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)) | "Removal of the GET stream endpoint" ([streamable-http 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)) |
| Tasks | Experimental core utility in 2025-11-25. Clients may declare the capability | Moved to the extension `io.modelcontextprotocol/tasks`: the client polls `tasks/get` and answers `tasks/update`, and "servers [may] return task handles unsolicited" (SEP-2663, [tasks overview](https://modelcontextprotocol.io/extensions/tasks/overview)) |
| Sessions | `Mcp-Session-Id` | Protocol-level sessions are removed (SEP-2567). There is no `initialize`; `server/discover` replaces it |

No revision has any clause about putting notification content into the model's context or starting a model turn.

---

## 2. Matrix

Each cell gives yes, no, partial or unknown, plus the levels reached. "—" means not applicable.

| Client (version / date checked) | 1 subscribe + `resources/updated` | 2 `list_changed` | 3 `notifications/message` | 4 long-poll tool + progress (timeout; does progress reset it?) | 5 sampling / elicitation | 6 GET SSE | 7 MCP Tasks | 8 vendor push / hooks | **Idle wake (c) from outside?** |
|---|---|---|---|---|---|---|---|---|---|
| **Claude Code CLI** (docs 2026-10-26; v2.1.310, 2026-10-23) | **no** (#7252 closed not-planned). Resources only via `@`-mention or resource tools (b, on demand) | **yes (a)**: refetches tools, prompts and resources (only tools in `-p`/SDK). Effect is on the next turn only | unknown (not documented) | **yes (b)**. Wall-clock limit `timeout` / `MCP_TOOL_TIMEOUT` (≈28 h default) is *not* extended by progress. Idle timeout (5 min HTTP, 30 min stdio) *is* reset by progress. Calls auto-background after 2 min and return their result as a task notification. Idle wake for that path: [INFERENCE] | sampling **no** (#1785 open); elicitation **yes (a, user dialog)** | [INFERENCE] yes on the v1 runtime (TS SDK 1.x opens GET after `initialized`); on 2026-07-28 it opens `subscriptions/listen` (#91414) | **yes (a,b,c)**: tasks extension on 2026-07-28, public beta v2.1.304+. Idle → "starts on it unprompted" | **channels** (a,b; c disputed), **asyncRewake hooks** (c), **Monitor / plugin monitors** (b, c), `mcp_tool` hooks (b), Stop hook block (continue at turn end), cross-session inbox socket (c) | **yes**, through Claude-Code-specific features |
| **Codex CLI / IDE / app** (openai/codex main@970111f6, 2026-10-23) | **no**. Never subscribes; `resources/updated` is only logged (#16159 open) | resources/prompts: log only. Tools: (a), behind the under-development flag `mcp_tool_list_changed` | (a): tracing log file only | `tool_timeout_sec` default **60 s**, a fixed deadline; **progress does not reset it** (progress is only logged) | sampling **no**; elicitation **yes (a)**, only during a tool call | unknown ([INFERENCE] rmcp may open it; it would reach the log handler only) | **no** (#48617 open; client pins 2025-06-18) | hooks (`SessionStart`, `UserPromptSubmit`, `PostToolUse`, `Stop`…): developer context (b), `Stop` block (continue at turn end); `notify` is outbound only; app-server `thread/inject_items` / `turn/start` (host-driven) | **no** (only an app-server host can start a turn) |
| **Cursor IDE** (docs + staff forum posts, 2.x/3.x, 2026) | **partial (a)**: sends `resources/subscribe` (even when `subscribe:false`); no re-inject found | tools: **no** (IDE + CLI forum reports); resources/prompts: unknown | (a): Output › "MCP Logs" | IDE: idle timeout **reset by progress since Jul 2026** (value not published). CLI/ACP: fixed **60 s**, no reset | sampling **no**; elicitation **yes (a)** (since 1.5, 2025-08) | unknown | no evidence | hooks: `postToolUse`/`sessionStart` `additional_context` (b); `stop`/`subagentStop` `followup_message` auto-submits the next user message (continue at turn end, `loop_limit` 5) | **no** |
| **VS Code Copilot, Local sessions** (microsoft/vscode main, 2026-10-26) | **partial (a)**: subscribes only for resources watched as virtual files; no model injection | **yes (a)** resources/tools/prompts | (a): server output log | progress **shown in chat UI**. No MCP-layer timeout found (higher layer not checked) | **yes / yes (a)**, user-facing | **yes**: GET with `Last-Event-ID` and retry | **yes**: 2025-11-25 tasks (VS Code 1.107) | hooks (preview): `sessionStart`, `postToolUse` add context (b); `agentStop` (turn end) | **no** |
| **VS Code, Copilot-runtime sessions** | unknown ([INFERENCE] the Copilot runtime is the client) | unknown | unknown | unknown | unknown | unknown | unknown | same hooks | unknown / no evidence |
| **Gemini CLI** (google-gemini/gemini-cli main, 2026-10-23) | **no** (`@server://…` read on user request only) | **yes (a)** tools/resources/prompts, even when not declared | **no handler** ([INFERENCE] dropped) | `timeout` default **600 000 ms**; progress displayed, **no reset** | **no / no** (only `roots/list` handled) | [INFERENCE] likely (TS SDK transport) | no evidence | hooks: `additionalContext` (b); `AfterAgent` deny+reason → "sent to the agent as a new prompt" (turn end) | **no** |
| **Claude Desktop / claude.ai** (connector docs, 2026-10-26) | **no**: "Resource subscriptions" listed as unsupported | unknown (#88172: Desktop keeps a stale `tools/list`) | unknown | **240 s** per tool call; reset unknown | sampling **no**; elicitation unknown (#88901: embedded build doesn't render it) | unknown | [INFERENCE] no ("Advanced or draft capabilities" unsupported) | none found | **no** |
| **Goose** (block/goose main, 2026-10-26) | no | tools (a); others unknown | (a): fanned out to UI/event subscribers | default extension `timeout` **300 s**; progress forwarded; reset unknown | sampling no; elicitation yes (a) | unknown | unknown | not researched | no evidence |
| **Zed** (docs) | no (resources unsupported) | tools only (a) | unknown | unknown | no / no | unknown | unknown | unknown | no evidence |
| **Windsurf / Devin Desktop (Cascade)** (docs) | unknown | tools (a), per the third-party canimcp crawl | unknown | not documented | unknown | unknown | unknown | hooks can only block or log; no context injection or follow-up | **no** |
| **Cline** (repo/docs) | unknown | tools (a), per canimcp | unknown | per-server `timeout`, default 60 s (third-party source) | unknown | unknown | unknown | hooks `contextModification` (b, next request); no follow-up; hooks unsupported on Windows | **no** |

---

## 3. Claude Code (CLI), in depth

Docs fetched 2026-10-26. The current version is v2.1.310, released 2026-10-23 ([changelog](https://code.claude.com/docs/en/changelog)).

### 3.1 Channels: the purpose-built push mechanism

**What it is.** Per the [channels docs](https://code.claude.com/docs/en/channels):

- "A channel is an MCP server that pushes events into your running Claude Code session, so Claude can react to things that happen while you're not at the terminal."
- The comparison table states the contrast directly: Standard MCP server: "Claude queries it during a task; **nothing is pushed to the session**."

**Timeline** ([changelog](https://code.claude.com/docs/en/changelog)):

| Version | Date | Change |
|---|---|---|
| 2.1.80 | 2026-03-19 | "Added `--channels` (research preview) — allow MCP servers to push messages into your session" |
| 2.1.81 | — | Permission relay |
| 2.1.83 | — | "Disabled `AskUserQuestion` and plan-mode tools when `--channels` is active" |
| 2.1.128 | 2026-05-04 | "`--channels` now works with console (API key) authentication" |
| 2.1.234 | — | Permission requests go only to servers registered as channels |

#### Server contract

From the [channels reference](https://code.claude.com/docs/en/channels-reference):

1. **Capability:** "Declare the `claude/channel` capability so Claude Code registers a notification listener."
   ```ts
   capabilities: { experimental: { 'claude/channel': {} },  // required, always {}
                   tools: {} },                              // only for two-way (reply tool)
   instructions: 'Messages arrive as <channel source="your-channel" ...>. Reply with the reply tool.'
   ```
   - The optional `experimental['claude/channel/permission']: {}` asks Claude Code to relay tool-approval prompts to the channel.
   - "Before v2.1.234, Claude Code treated `false` as declared."
2. **Notification:** method `notifications/claude/channel` with params:
   ```json
   { "jsonrpc":"2.0", "method":"notifications/claude/channel",
     "params": { "content": "build failed on main: …",
                 "meta": { "severity": "high", "run_id": "1234" } } }
   ```
   - `content` is a string and becomes the body of the tag.
   - `meta` is an optional `Record<string,string>`. "Keys must be identifiers: letters, digits, and underscores only. Keys containing hyphens… are silently dropped."
   - The model sees: `<channel source="your-channel" severity="high" run_id="1234">build failed on main: …</channel>`
3. **Transport:** "Connect over stdio transport." Also: "Claude Code spawns it as a subprocess and communicates over stdio."
   - No HTTP or remote channel transport is documented.
   - An issue search for channel-over-HTTP turned up nothing.
4. **Delivery semantics:**
   - "Claude Code doesn't acknowledge notifications… If the session hasn't loaded your server as a channel, or the organization policy blocks it, Claude Code **drops the events silently**."
   - "Events queue into the session and are processed in order. If several notifications arrive while Claude is busy, they're delivered together on the next turn."

#### Enabling a channel (user side)

**Development or custom server, not on the allowlist:**

```jsonc
// .mcp.json
{ "mcpServers": { "mailbus": { "command": "mailbus-channel", "args": [] } } }
```
```bash
claude --dangerously-load-development-channels server:mailbus
# plugin form: --dangerously-load-development-channels plugin:<name>@<marketplace>
```

**Allowlisted plugins:** `claude --channels plugin:<name>@<marketplace>`. Several entries can be passed, separated by spaces.

**As a plugin:** the manifest needs a `channels` entry bound to one of its `mcpServers`. The key is `"channels": [{ "server": "telegram", "userConfig": {…} }]` ([plugin components](https://code.claude.com/docs/en/plugins/components)).

#### Constraints ([channels](https://code.claude.com/docs/en/channels), [reference](https://code.claude.com/docs/en/channels-reference), [mcp](https://code.claude.com/docs/en/mcp#push-messages-with-channels))

- **Research preview:** "the `--channels` flag syntax and protocol contract may change". The flags don't appear in `claude --help`.
- **Auth:**
  - Requires "Anthropic authentication through claude.ai or a Console API key".
  - Not available on Bedrock, Google Cloud Agent Platform or Foundry.
  - Team and Enterprise: blocked until an Owner sets `channelsEnabled: true`. `allowedChannelPlugins` replaces the Anthropic allowlist.
  - Pro and Max users without an organization skip these checks.
- **Allowlist:**
  - "During the research preview, `--channels` only accepts plugins from an Anthropic-maintained allowlist." Custom channels need `--dangerously-load-development-channels`.
  - That flag shows "a full-screen warning dialog" at startup.
  - "Being in `.mcp.json` isn't enough to push messages: a server also has to be named in `--channels`."
- **Interactive only:**
  - "If you pass it in non-interactive mode with `-p` or through the Agent SDK, Claude Code ignores the flag and the channel doesn't register."
  - #99658 (open, 2026-10-05): `-p` never delivers channel events.
- **Protocol pinning gotcha:**
  - "On the v2 runtime, a channel server that negotiates MCP protocol revision 2026-07-28 can't deliver channel messages, so Claude Code doesn't register it as a channel… Claude Code asks stdio servers for that revision by default. To keep a stdio channel server on the earlier handshake, set `MCP_PROTOCOL_NEGOTIATION` to `legacy`."
  - #97173 (open, 2026-09-25) shows the regression this causes: since 2.1.282, "proactive server push breaks silently", with the message "this connection's protocol version has no channel delivery path".
  - **go-sdk v1.8.0 negotiates 2026-07-28 by default**, so a Go stdio channel server must restrict the versions it offers with `ServerOptions.SupportedProtocolVersions`, which go-sdk v1.8.0 provides ([release notes](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)). The user can set `MCP_PROTOCOL_NEGOTIATION=legacy` instead.
- **go-sdk can't emit the notification:**
  - Verified in v1.8.0 source: `ServerSession` has no public generic-notify API (only `NotifyProgress`, `Log`, `NotifyElicitationComplete`, …).
  - `defaultSendingMethodHandler` in `mcp/shared.go` returns `jsonrpc2.ErrNotHandled` for any method not in `sendingMethodInfos()`.
  - So `notifications/claude/channel` can't be sent through the SDK. [INFERENCE] A channel shim must be written in TypeScript with `@modelcontextprotocol/sdk` (the documented path) or speak raw JSON-RPC over stdio.
- **Mid-turn framing:** #93314 (open, 2026-09-10) reports that `<channel>` messages arriving mid-turn get an "untrusted external data" wrapper.

#### Idle wake: documented, but disputed by the open bug #44380

- The docs imply idle sessions wake. Quickstart: the message "arrives in your Claude Code session… Claude reads it, does the work." Webhook example: "You'll then see Claude start responding."
- [#44380 "Channel messages don't wake idle sessions"](https://github.com/anthropics/claude-code/issues/44380) is **open**:
  - Opened 2026-04-06 against v2.1.92. Independent confirmations followed through 2026-09, including a custom stdio channel on 2026-06-21.
  - One reporter's binary analysis (2026-06-03, v2.1.161) says the notification is enqueued with `priority:"next"` but not flushed while the REPL is idle.
  - No maintainer response.
  - Related open issues: #36477 and #36431 (2026-03), #78028 (development flag lost on daemon fork-resume), #101872 (notifications dropped after handoff to a background session).
  - No changelog entry up to v2.1.310 claims a fix.
- Contrary data point: #99658 (2026-10-05, v2.1.287–289, interactive) says "a pushed event reaches the transcript as a channel-origin user turn within a second". That report doesn't state whether the session was idle.
- **Net:** (a) yes and (b) yes. **(c) is unreliable.** Treat it as unverified until tested on the target version.

### 3.2 Hooks

All of these are configured by the user in `settings.json` or a plugin's `hooks/hooks.json`; an MCP server can't register them ([hooks reference](https://code.claude.com/docs/en/hooks)).

#### `asyncRewake`: the documented idle-wake path

- Field table: "`asyncRewake`: If `true`, runs in the background and **wakes Claude on exit code 2**. The hook's stderr, or stdout if stderr is empty, is shown to Claude as a system reminder."
- Limitations: "If the session is idle, the response waits until the next user interaction. **Exception: an `asyncRewake` hook that exits with code 2 wakes Claude immediately even when the session is idle.**"
- Timeouts: "Claude Code still enforces `timeout` on a hook you run with `asyncRewake`." The default `timeout` for command hooks is 600 s and can be set per hook.
- Command hooks only. v2.1.287 (2026-10-01) fixed `asyncRewake` hooks "waking Claude over and over… when the hook's script file is missing" ([changelog](https://code.claude.com/docs/en/changelog)).
- Community repro on #44380 (2026-07-26, v2.1.211): a `Stop` hook with `asyncRewake: true` polls a flag file and runs `exit 2` → "The session starts a turn on its own… 3.1 s later". The comment also says the official `security-guidance` plugin ships `asyncRewake`.

Mail-bus pattern [INFERENCE]:

```json
{ "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "mailbus-wait", "asyncRewake": true, "timeout": 86400 }] }],
    "Stop":         [{ "hooks": [{ "type": "command", "command": "mailbus-wait", "asyncRewake": true, "timeout": 86400 }] }] } }
```

- `mailbus-wait` long-polls the Go server's HTTP endpoint, for example `curl`.
- On mail it prints the mail to stderr and exits 2. On timeout it exits 0.
- Each `Stop` re-arms it. Use a single-flight lock, as in the repro.
- Whether a very large `timeout` is capped is unknown.

#### `mcp_tool` hooks

- `"type": "mcp_tool"` "call[s] a tool on a configured MCP server. The tool's text output is treated like command-hook stdout."
- Fields: `server`, `tool`, `input`.
- So a user-configured hook can call the bus's own `check_mail` tool without the model doing so, for example on `UserPromptSubmit`, `PostToolUse` or `Stop`. The result reaches level (b) through `additionalContext`, or continues the turn on `Stop` with `decision:"block"`.
- No wake: `async` and `asyncRewake` exist only on command hooks.

#### Context injection (level b)

- `UserPromptSubmit`: stdout or `additionalContext` "added to Claude's context". The default timeout here is 30 s. It also fires on "turns Claude Code starts on its own": scheduled tasks and `/loop`, background subagents, and cross-session messages.
- `PostToolUse` `additionalContext`.
- `SessionStart` `additionalContext`.
- Async hooks deliver `additionalContext` "on the next conversation turn".

#### `Stop` (continue at turn end)

- `decision: "block"` + `reason`, or `hookSpecificOutput.additionalContext`, keeps the conversation going.
- Loop protections apply: the `stop_hook_active` input and an "8-consecutive-continuation cap".

#### `Notification` hook

- It is only an outward signal: `idle_prompt` fires "about 60 seconds after Claude finishes responding".
- No decision control.

### 3.3 Monitor tool and plugin monitors (each output line becomes a notification)

**Monitor tool** (v2.1.98, 2026-04-09). From the [what's-new page](https://code.claude.com/docs/en/whats-new/2026-w15): "spawns a background watcher and streams its events into the conversation: each event lands as a new transcript message that Claude reacts to immediately."

Limits ([tools reference](https://code.claude.com/docs/en/tools-reference#monitor-tool)):

- "Every watch Claude starts has a deadline: 5 minutes by default, at most 30 minutes."
- WebSocket source: `ws` URLs. "Claude Code denies URLs that point at a private, link-local, or cloud-metadata address."
- Windows needs Git Bash.

**Plugin monitors** ([plugin components](https://code.claude.com/docs/en/plugins/components#monitors)): "a shell command that Claude Code starts in the background when the session starts and keeps running until it ends. What it prints reaches Claude as notifications."

```json
// <plugin>/monitors/monitors.json   (manifest key: experimental.monitors)
[{ "name": "mailbus", "command": "curl -sN http://127.0.0.1:8080/mail/stream", "description": "Agent mailbox" }]
```

- "plugin monitors don't start in non-interactive mode with the `-p` flag."
- No documented deadline for plugin monitors.
- Idle wake [INFERENCE]: likely, given "reacts to immediately". The docs don't state the idle case explicitly.

### 3.4 Cross-session messaging inbox (a documented way for scripts to post into a session)

- Requires v2.1.224+, or v2.1.234+ on native Windows.
- "When the receiving session is idle, Claude Code starts a new turn with the message" ([cross-session messaging](https://code.claude.com/docs/en/cross-session-messaging)).
- Each session binds an inbox socket (a named pipe on Windows), exported to hooks and Bash as `CLAUDE_CODE_MESSAGING_SOCKET` with the token `CLAUDE_CODE_MESSAGING_TOKEN`.
- The docs name this case: "when you want a script or hook to post into a session".
- On Windows the first line must be `{"type":"auth","token":"<token>"}`.
- Messages go through the receiving session's inbound controls (`crossSessionInbound`: accept, hold or refuse).
- The wire format after the auth line isn't documented. [INFERENCE] It is usable only by a local helper, not by the remote Go server directly.

### 3.5 Long-poll tool behaviour ([mcp#time-limits](https://code.claude.com/docs/en/mcp#time-limits-on-mcp-tool-calls))

- **Wall-clock limit:** "The per-server `timeout` is a hard wall-clock limit per tool call, and **progress notifications from the server don't extend it**."
  - If unset, it falls back to `MCP_TOOL_TIMEOUT`, which defaults to "about 28 hours".
- **Per-request timer** for HTTP servers: covers each request "through to the server's first response byte", set to max(60 s, tool timeout, `MCP_TIMEOUT`).
- **Idle timeout:** "A tool call… that sends no response **and no progress notification** for the idle window aborts".
  - Window: 5 min for HTTP, 30 min for stdio.
  - Configured with `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` (`0` disables it).
  - So a blocking `wait_for_mail` must send `notifications/progress` at least every few minutes.
- **Auto-background** (v2.1.212, 2026-07-17): "An MCP tool call in the main conversation that is still running after two minutes moves to a background task… the result arrives as a task notification when the call settles."
  - Disable with `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0`.
  - Since v2.1.283 the background task shows the latest progress.
  - [INFERENCE] The docs don't say whether a background-call result wakes an idle session.

### 3.6 MCP Tasks (an official tasks extension with idle wake)

From [mcp#mcp-tasks-that-run-on-the-server](https://code.claude.com/docs/en/mcp#mcp-tasks-that-run-on-the-server):

- "When the job ends, the transcript says so and Claude gets the result… **If Claude is idle when the result arrives, Claude starts on it unprompted.**"

**Requirements:**

- "public beta in the Claude Code CLI and require Claude Code v2.1.304 or later".
- The server supports tasks and is connected on protocol revision 2026-07-28.
- The session fetches feature flags, so not on Bedrock.
- Terminal CLI, Agent SDK or GitHub Actions. "VS Code extension and the desktop app don't have MCP tasks".
- Main conversation only, not subagents.

**Wire contract for server authors:**

- Answer `tools/call` with `{"resultType":"task","taskId":…,"status":"working","createdAt":…,"lastUpdatedAt":…,"ttlMs":null,"pollIntervalMs":5000}`.
- Claude Code then polls `tasks/get`. `pollIntervalMs` is clamped to 250–30000 ms; the default is 2000.
- When finished, answer `status:"completed"` with the `tools/call` result.

**Applied to the mail bus:** this makes `wait_for_mail` a task that completes when mail arrives. That wakes an idle session from a pure HTTP MCP server, but only on Claude Code CLI.

**go-sdk v1.8.0 has no tasks support:**

- No `tasks/get` handling exists in `mcp/`.
- `ROADMAP.md` lists "SEP-1686 (experimental): Implement Tasks" ([go-sdk#626](https://github.com/modelcontextprotocol/go-sdk/issues/626)).
- [INFERENCE] It would need custom middleware or an SDK upgrade.

### 3.7 Other Claude Code MCP behaviour

- **list_changed:** "In an interactive terminal session, Claude Code fetches the updated list… In non-interactive mode… refreshes only the tool list" ([mcp#dynamic-tool-updates](https://code.claude.com/docs/en/mcp#dynamic-tool-updates)).
  - On the v2 runtime it receives these "over a stream it holds open", meaning `subscriptions/listen`.
  - That stream is reopened at most 3 times when it closes within 10 s. If it closes after staying open longer, Claude Code waits about 6 h after 5 reopens in an hour.
  - #91414 (open): the first turn can block about 25 s waiting for `subscriptions/listen` on HTTP 2026-07-28 servers.
- **Resources:** `@server:proto://path` mentions plus automatic list and read tools. No subscription is documented. Feature request [#7252](https://github.com/anthropics/claude-code/issues/7252) ("support MCP resource subscriptions for automatic context updates") was closed not-planned by the stale bot on 2026-01-05.
- **Elicitation:** dialog shown to the user (a), with an `Elicitation` hook to auto-answer. On 2026-07-28 it declares `elicitation: {form: {}, url: {}}`. **Sampling:** not documented; [#1785](https://github.com/anthropics/claude-code/issues/1785) is open (2025-06-08).
- **WebSocket transport** (`"type":"ws"`) is "suited to remote MCP servers that push events to Claude unprompted" ([mcp option 4](https://code.claude.com/docs/en/mcp#option-4-add-a-remote-websocket-server)). [INFERENCE] This only says the connection can carry server-initiated messages. Nothing documents those messages reaching the model, beyond channels, which are stdio-only.
- **Runtimes:**
  - v1 runtime: TS SDK 1.x.
  - v2 runtime: TS SDK 2.0 + 2026-07-28. It is the default from v2.1.232 with feature flags, or v2.1.274 without.
  - Override with `MCP_SDK_GENERATION=v1|v2` and `MCP_PROTOCOL_NEGOTIATION=auto|legacy`.
  - [INFERENCE] On v1, the TS SDK's `StreamableHTTPClientTransport` opens the GET SSE stream right after sending `notifications/initialized` ([typescript-sdk v1.x streamableHttp.ts L605-607](https://github.com/modelcontextprotocol/typescript-sdk/blob/v1.x/src/client/streamableHttp.ts)).
  - go-sdk v1.8.0: "a stateful streamable handler receiving a `2026-07-28` request now returns that same JSON-RPC error… so the client can renegotiate down" ([release notes](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)).
- **Scheduled tasks / `/loop`** ([scheduled-tasks](https://code.claude.com/docs/en/scheduled-tasks)): these are polling, not push.
  - Minimum interval 1 min. Recurring tasks expire after 7 days. Tasks "only fire while Claude Code is running and idle."
  - The doc itself points to Channels for "reacting to events as they happen instead of polling".
- **Related open feature requests:**
  - #90379 (2026-08-28): advisory-notes channel for background watchers.
  - #101434 (2026-10-12): start routines from MCP events.
  - No maintainer replies found.

---

## 4. Other clients

### 4.1 Codex CLI (OpenAI)

Sources: openai/codex `main@970111f6` (2026-10-23); [developers.openai.com/codex/mcp](https://developers.openai.com/codex/mcp), [config-reference](https://developers.openai.com/codex/config-reference), [hooks](https://developers.openai.com/codex/hooks) and [app-server](https://developers.openai.com/codex/app-server), all fetched 2026-10-26.

**Resources:**
- `codex-rs/rmcp-client/src/logging_client_handler.rs` `on_resource_updated` only does `info!("MCP server resource updated (uri: {})", …)`.
- Codex never sends `resources/subscribe`.
- [#16159](https://github.com/openai/codex/issues/16159) (open, 2026-03-29) marks subscribe "❌ Not implemented" and updated "⚠️ Received but only logged".
- A commenter there describes an agent-inbox bridge that has to "poll resources/list + resources/read on a 30s loop". That is our use case.

**list_changed:**
- Resources and prompts: logged only.
- Tools: a catalog refresh exists in `codex-mcp/src/connection_manager.rs`, but only behind the feature `mcp_tool_list_changed` (`Stage::UnderDevelopment`).
- Open issues: [#10105](https://github.com/openai/codex/issues/10105), [#33266](https://github.com/openai/codex/issues/33266), [#37417](https://github.com/openai/codex/issues/37417).

**Logging:** `on_logging_message` writes to the tracing log only. It isn't shown in the TUI or to the model.

**Timeouts:**
- "`tool_timeout_sec`… Default: `60`". `startup_timeout_sec` defaults to 10.
- `codex-mcp/src/binding.rs` sets `timeout_deadline = Instant::now() + timeout` and calls `tokio::time::timeout_at`, which is a **fixed deadline**.
- `on_progress` only logs, so **progress doesn't extend it**.

**Sampling and elicitation:**
- No sampling capability is declared. PR [#5116](https://github.com/openai/codex/pull/5116) was closed unmerged.
- Elicitation is declared and forwarded to the user during a call. [#39149](https://github.com/openai/codex/issues/39149) reports it can hang.

**Tasks:** none. The client pins `ProtocolVersion::V_2025_06_18`. [#48617](https://github.com/openai/codex/issues/48617) is open.

**Hooks and notify:**
- `notify` = "Command invoked for notifications; receives a JSON payload from Codex". It is outbound only.
- Hook events: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, …
- "Plain text on `stdout` is added as extra developer context" (b).
- `Stop` with `decision:"block"` forces continuation at turn end.
- Nothing wakes an idle session.

**App-server:**
- `thread/inject_items` "Append[s] raw Responses API items to the thread history without starting a user turn".
- `turn/start` starts a turn.
- The experimental `mcpServer/event/stream/*` forwards MCP notifications to the embedding host.
- [INFERENCE] Only a custom host embedding the app-server can turn MCP events into turns.

### 4.2 Cursor

Sources: [cursor.com/docs/mcp](https://cursor.com/docs/mcp), [cursor.com/docs/hooks](https://cursor.com/docs/hooks) (fetched 2026-10-26), and forum threads with staff replies.

**Resources:**
- Supported ([changelog](https://cursor.com/changelog/page/14)).
- Cursor sends `resources/subscribe` even when the server declares `subscribe:false`. Staff, 2026-02-13, v2.4.31: "Cursor is currently ignoring the server-declared capability `subscribe: false` and still tries to send `resources/subscribe`" ([forum 151758](https://forum.cursor.com/t/mcp-resources-support/151758)).
- No evidence that an updated resource reaches the model.

**Tools list_changed is not acted on:**
- [forum 161459](https://forum.cursor.com/t/mcp-notifications-tools-list-changed-not-acted-on-mid-session/161459) (CLI, 2026-05): "the tool list is never re-fetched… until the session is restarted".
- Older IDE reports: [77294](https://forum.cursor.com/t/mcp-client-update-tools/77294), [132363](https://forum.cursor.com/t/mcps-tool-list-changed-not-picked-up/132363).

**Logging:** Output panel → "MCP Logs" (user only).

**Timeouts:**
- Staff, 2026-10-02: "The IDE MCP client got a progress-aware timeout in July. The idle timeout resets when it gets notifications or progress."
- The CLI/ACP client still uses "the SDK's 60s default timeout… and it ignores progress" ([forum 160548/9](https://forum.cursor.com/t/acp-mcp-client-should-reset-json-rpc-tool-call-timeout-on-notifications-progress-or-honour-resettimeoutonprogress/160548/9)).
- Progress isn't rendered in chat as of 3.13.25 ([forum 166948](https://forum.cursor.com/t/mcp-tool-progress-notifications-not-shown-in-chat-ui-notifications-progress-cursor-3-13-25-stable/166948)).

**Sampling and elicitation:** elicitation is supported since [1.5](https://cursor.com/changelog/1-5) (2025-08-21); sampling is absent from the docs feature table.

**Hooks** ([cursor.com/docs/hooks](https://cursor.com/docs/hooks)):
- `postToolUse` `additional_context`: "Extra context injected into the conversation after the tool result".
- `sessionStart` `additional_context`.
- `stop`: "The optional `followup_message`… Cursor will automatically submit it as the next user message".
- `loop_limit` "Default is `5`… `null` means no limit".
- Cloud agents get no user-level hooks.
- [INFERENCE] A `stop` hook could long-wait on the bus and auto-continue, but only at the end of a turn. It can't wake a session that is already idle.

### 4.3 VS Code GitHub Copilot (agent mode)

Sources: microsoft/vscode main (2026-10-26), `src/vs/workbench/contrib/mcp/common/` and `src/vs/workbench/api/common/extHostMcp.ts`; docs at [code.visualstudio.com/docs/agent-customization/mcp-servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers).

**Caveat:** "Copilot"-runtime sessions are forwarded the servers instead of using the editor client. "The agent doesn't read `.vscode/mcp.json` directly" ([mcp-configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)). The findings below are for **Local** sessions.

**Resources:**
- `mcpServerRequestHandler.ts` sends `resources/subscribe`.
- The only caller is `mcpResourceFilesystem.ts`, which subscribes to resources watched as virtual files and turns updates into file-change events.
- Resources reach chat only through manual "Add Context > MCP Resources".
- [#251870](https://github.com/microsoft/vscode/issues/251870) (automatic resource selection) was closed not_planned.

**list_changed:** resources, tools and prompts trigger refreshes ([#243944](https://github.com/microsoft/vscode/issues/243944)).

**Logging:** `translateMcpLogMessage` writes to the server output log.

**Progress and timeout:**
- `mcpServer.ts` maps progress to `progress.report` in the chat UI.
- `sendRequest` takes only a `CancellationToken`; no MCP-layer timeout was found. [INFERENCE] A blocking tool runs until the user cancels it. The higher `languageModelToolsService` layer wasn't checked.

**Sampling and elicitation:** both handled (`sampling/createMessage`, `elicitation/create`), and both are user-facing.

**GET SSE:** `extHostMcp.ts`: "Attaches the SSE backchannel that streamable HTTP servers can use for async notifications… if the server gives us a 4xx code, we'll stop trying". It retries with backoff and `Last-Event-ID`.

**Tasks:** 2025-11-25 tasks are supported for `tools/call` (polls `tasks/get`). Release [1.107](https://code.visualstudio.com/updates/v1_107) covers 2025-11-25 support.

**Hooks (preview)** ([hooks](https://code.visualstudio.com/docs/agent-customization/hooks), [GitHub hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference)):
- `sessionStart` and `postToolUse` can add context.
- `agentStop` "Check[s] whether the agent should finish its turn".
- `userPromptSubmitted` output is ignored.
- No idle wake.

### 4.4 Gemini CLI

Sources: google-gemini/gemini-cli main (2026-10-23): [packages/core/src/tools/mcp-client.ts](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/tools/mcp-client.ts), [docs/tools/mcp-server.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md), [docs/hooks/reference.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/reference.md).

- **list_changed:** handlers for tools, resources and prompts. The client listens even when the server didn't declare it ("did not declare 'listChanged' capability. Listening anyway for robustness…", L415/437/459).
- **Resources:** no subscribe handler. Resources are injected only when the user writes `@server://resource/path`.
- **Logging:** no `notifications/message` handler.
- **Timeout:** `MCP_DEFAULT_TIMEOUT_MSEC = 10 * 60 * 1000` (L99). Progress goes to `coreEvents.emitMcpProgress` (L481) for display. No `resetTimeoutOnProgress`.
- **Sampling and elicitation:** none. The only client request handler is `ListRootsRequestSchema` (L1886).
- **Hooks:**
  - `BeforeAgent`, `AfterTool` and `SessionStart` take `additionalContext` ("Text that is **appended**").
  - `AfterAgent`: `decision:"deny"` plus `reason`, "This text is sent **to the agent as a new prompt**", gives continuation at turn end.
  - No idle wake.

### 4.5 Claude Desktop / claude.ai

Source: [claude.com/docs/connectors/building](https://claude.com/docs/connectors/building), fetched 2026-10-26.

- "Claude doesn't yet support these MCP features, so don't build a feature that depends on them: **Resource subscriptions; Sampling; Advanced or draft capabilities**."
- "Tool call timeout: **240 seconds** per tool call" on claude.ai and Desktop.
- Known bugs in the embedded Claude Code (Desktop/Cowork):
  - [#88901](https://github.com/anthropics/claude-code/issues/88901): server-initiated elicitations are not rendered.
  - [#88172](https://github.com/anthropics/claude-code/issues/88172): stale `tools/list`.
- Channels and MCP tasks are documented as CLI features. The desktop app has no MCP tasks.

### 4.6 Others (brief)

- **Goose** (block/goose main, 2026-10-26, `crates/goose/src/agents/mcp_client.rs`):
  - Handles `on_tool_list_changed`, `on_logging_message` (fanned out to subscribers) and `on_progress`.
  - Capabilities: roots, extensions, elicitation; no sampling.
  - `DEFAULT_EXTENSION_TIMEOUT` is 300 s.
  - No subscription handling; no wake found.
- **Zed** ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp)): "Zed currently supports MCP's Tools and Prompts features… (Discovery, Sampling, Elicitation, etc)" are not supported. It "handles the `notifications/tools/list_changed` notification".
- **Windsurf / Devin Desktop (Cascade)** ([mcp](https://docs.devin.ai/desktop/cascade/mcp), [hooks](https://docs.devin.ai/desktop/cascade/hooks)):
  - Hooks can only block (exit 2) or `show_output` to the UI.
  - No context injection, no follow-up.
- **Cline** ([hooks README](https://github.com/cline/cline/blob/main/.clinerules/hooks/README.md)):
  - `contextModification` is "added to the conversation… AI sees this context in the NEXT API request".
  - No follow-up or wake.
  - "Windows: Not currently supported".
  - The per-server `timeout` default of 60 s comes from a third-party source.

---

## 5. What this means for the Go mailbus [INFERENCE]

| Goal | Portable (any client) | Claude Code only |
|---|---|---|
| Model sees mail during a turn (b) | A long-poll `wait_for_mail` / `check_mail` tool. Keep each call under the client's timeout: Codex 60 s, Cursor CLI 60 s, Desktop 240 s, Goose 300 s, Gemini 600 s. Send `notifications/progress` regularly: it resets the timeout in Claude Code (idle window) and the Cursor IDE, but not in Codex or Gemini | Also: `mcp_tool` hooks on `UserPromptSubmit` / `PostToolUse` that call the bus's tool, so the model never has to |
| Keep a finishing agent going while mail is pending | Client hooks: Cursor `stop.followup_message`, Codex `Stop` block, Gemini `AfterAgent` deny, Copilot `agentStop`. All are user-configured | `Stop` hook `decision:"block"`, including an `mcp_tool` hook calling the bus |
| Idle session wakes on new mail (c) | **Not possible from a pure MCP server** in any researched client | 1) `asyncRewake` command hook that long-polls the bus over HTTP (documented, community-verified). 2) Plugin monitor streaming `curl -N` from a bus SSE or long-poll endpoint. 3) A stdio channel shim (TS SDK; pin protocol ≤ 2025-11-25; development flag; research preview; idle wake disputed by #44380). 4) MCP Tasks on 2026-07-28 (needs tasks-extension support that go-sdk v1.8.0 lacks). |

Notes for the HTTP server itself:

- **Listen stream:** since Claude Code v2 opens `subscriptions/listen` on 2026-07-28 HTTP servers, answer it promptly. Per #91414, the first turn can otherwise stall for up to `MCP_TIMEOUT − 5 s`.
- **GET stream:** on older protocol versions, VS Code and the TS-SDK-based clients will open the GET SSE stream, but nothing they receive on it reaches the model.

---

## 6. Verdict on the PO claim

> "Submitting events into a running session is not that easy via a pure MCP server."

**Largely correct, with one qualification.**

**Why it holds:**

- Spec-standard push (`resources/updated`, `list_changed`, logging, the GET stream) never reaches the model or wakes a session in any client checked: Claude Code, Codex, Cursor, VS Code, Gemini CLI, Claude Desktop, Goose, Zed, Windsurf, Cline.
- The spec requires none of it.
- The 2026-07-28 revision drops the GET stream and limits push to list and resource change notifications.

**Qualifications:**

1. **During an active turn, a pure MCP server can deliver mail** as the result of a long-poll tool call, at level (b). This works everywhere, subject to each client's timeout and progress rules (§2, column 4).
2. **Claude Code is the exception for real push, but none of its paths is "pure, standard MCP":**
   - **Channels:** a vendor `experimental` capability plus the custom `notifications/claude/channel` method. Stdio only, research preview, per-session `--channels` or a development flag with a startup dialog, claude.ai or Console auth (no Bedrock, Vertex or Foundry), org opt-in, protocol pinning. The idle-wake bug is still open.
   - **Hooks and monitors:** user or plugin configuration on the client, not something the server can enable.
   - **MCP Tasks:** an official MCP extension that does wake idle sessions. It needs protocol 2026-07-28, Claude Code CLI v2.1.304+ in public beta, and tasks-extension support that go-sdk v1.8.0 doesn't have.
