# agent-message-bus (omp extension)

Connects an omp session to an [agent-message-bus](../../../apps/agent-message-bus/README.md) server. The extension:

- registers the server's 29 `bus_*` tools in the session;
- streams the agent's inbox into the session as `[agent-bus]` messages, then acknowledges each one;
- adds the `/bus` command to manage credentials.

Server behaviour (spaces, topics, blackboards, permissions) is specified in [`apps/agent-message-bus/spec/README.md`](../../../apps/agent-message-bus/spec/README.md). Agent-facing usage guidance is in [`skills/agent-message-bus/SKILL.md`](../../../skills/agent-message-bus/SKILL.md).

Every tool's arguments, result shape and error codes are listed in [`apps/agent-message-bus/TOOLS.md`](../../../apps/agent-message-bus/TOOLS.md).

## Commands

| Command | Effect |
| :--- | :--- |
| `/bus login <url> <token>` | Calls `bus_whoami` with the token. **Admin** token: stored as `adminToken`, nothing else starts; reply `Admin token stored. Run /bus promote <leader-name> to act as a leader.` A stored agent token is kept if the url is the same. **Leader/member** token: stored as `token`, the session's promoted leader token (if any) is dropped, then tools are registered and the inbox stream starts; reply `Logged in as <kind> <name>`. A stored admin token is kept only if the url is the same, so `/bus promote` never sends one server's admin token to another. A trailing `/` on the url is stripped. |
| `/bus promote <leader-name>` | Requires a stored admin token (otherwise `Run /bus login <url> <admin-token> first.`). Calls `bus_mint_leader` and restarts with the new leader token. The token is held in this session's memory only and never written to `credentials.json`; other and later sessions are unaffected. Reply `This session now acts as leader <name> (until it ends; run /bus promote again after a restart).` |
| `/bus` or `/bus status` | Calls `bus_whoami` with the active credentials (see precedence below); if there is no agent token, it uses the stored admin token. Prints `<url>: <kind> <name>; spaces: <name> (<role>), …`, or `Not logged in.` with usage. |
| `/bus logout` | Deletes the credentials file and drops the session's promoted leader token. If env `AGENT_BUS_URL` and `AGENT_BUS_TOKEN` are set, the session restarts on them; reply `Logged out; session still uses AGENT_BUS_URL/AGENT_BUS_TOKEN from the environment.` Otherwise it stops the inbox stream; reply `Logged out.` Registered tools stay registered either way. |
| `/bus help` | Prints usage. |

Server errors are shown as `<code>: <message>`.

Example:

```text
/bus login http://localhost:8080 <admin-token>
/bus promote lead
/bus status
http://localhost:8080: leader lead; spaces: none
```

## Credentials

- File: `<PI_CODING_AGENT_DIR or ~/.omp/agent>/agent-message-bus/credentials.json`, shape `{url, adminToken?, token?}`. `/bus promote` never writes it.
- `OMP_AGENT_BUS_DIR` replaces the whole directory (the file is `$OMP_AGENT_BUS_DIR/credentials.json`).
- The file is written with mode `0600` and existing files are chmod-ed to `0600`. Windows ignores the mode; the user-profile ACL protects it.
- A corrupt file fails with `Credential file <path> is unreadable. Run /bus login to rewrite it.`
- **Precedence:** the session's promoted leader token (`/bus promote`) wins. Next, if `AGENT_BUS_URL` **and** `AGENT_BUS_TOKEN` are both set, they win over the stored url and token. Otherwise the stored `token` is used. With only an admin token, the session has no tools and no inbox. The env override lets a leader start worker sessions on member tokens without touching the shared file:

  ```powershell
  $env:AGENT_BUS_URL = "http://localhost:8080"; $env:AGENT_BUS_TOKEN = "<member token>"; omp
  ```

## Tools

At session start (or on `/bus login` or `/bus promote`), the extension fetches `GET /v1/tools` once per session. It registers each tool under its server name, converting the JSON Schema to zod via `pi.zod`. A tool that cannot be registered (for example an unsupported schema) is logged as `agent-bus: tool <name> not registered: …`; the other tools still register. Each call uses the credentials that are active **at call time**, so `/bus promote` changes identity without re-registering. A tool result is the server's `result` as pretty-printed JSON. A server error is thrown as `<code>: <message>`. After `/bus logout` (without env credentials), the tools stay registered but calls fail with `Not logged in to the agent bus. Run /bus login <url> <token>.` until the next login. If the tool fetch fails, the inbox stream is not started either; the error is logged as `agent-bus: …` (or shown as the command reply), and the next `/bus login` retries.

## Inbox delivery

The extension holds `GET /v1/inbox/stream` open. For each `inbox` event, it sends a custom message (`customType: "agent-bus"`, displayed) and then calls `bus_inbox_ack {upTo: id}`. The ack permanently deletes the entry on the server. A failed ack is logged as `agent-bus: <message>` and the stream continues: reconnecting would redeliver the entry, and the next ack's higher `upTo` covers it.

```text
[agent-bus] <space> · <where> · from <sender>[ · CRITICAL]
<body>
```

| Entry | `deliverAs` | Starts a turn |
| :--- | :--- | :--- |
| `critical: true` (leaders only) | `steer`: interrupts the running turn | in `tui` and `rpc` modes |
| everything else | `followUp`: queued after the current turn | in `tui` and `rpc` modes |

In one-shot `print`/`json` modes (and when the mode is unknown), entries are added to the context without triggering a turn, because those modes run their own prompt right after `session_start`.

**Reconnect:** when the stream ends or fails, the extension waits 1 s, then doubles the wait up to 30 s. Each delivered entry resets the wait to 1 s. An `unauthenticated` (401) or `forbidden` (403) error stops the loop for good: the token was revoked, the member left, or the space was deleted. Run `/bus login` again to restart. Errors are logged as `agent-bus: <message>`. The stream stops at session shutdown and on `/bus logout`.

## One identity per agent

The server allows one live inbox stream per agent, so each agent identity belongs to exactly one session. When another session already streams this agent's inbox, the server answers `409 conflict`. The extension then logs once, `agent-bus: another session is already connected as <name>; inbox not delivered here` (the name comes from `bus_whoami`; if that fails, `as this agent`), and keeps retrying with the normal backoff. The inbox is delivered here once the other stream ends, for example when its session exits or crashes (the server notices a dead connection at its next heartbeat, within about 20 s). Once this session's stream has opened, a later refusal logs again. Tool calls are not restricted. Give every agent its own identity: `/bus promote` is session-scoped, and each worker gets its own member token via env.

## Deploy

From the repo root, preview first, then seed:

```powershell
python scripts/seed.py --dry-run
python scripts/seed.py
```

The default `--scope user` copies the package to `~/.omp/agent/extensions/agent-message-bus/` and the skill to the user skills directory. Use `--scope project --project <path>` to seed into `<project>/.omp/` instead. `--only extensions` or `--only skills` limits what is seeded. Existing files are only overwritten with `--force`.
