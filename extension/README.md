# agent-message-bus (omp extension)

Connects an omp session to an [agent-message-bus](../server/README.md) server. The extension:

- registers the server's 29 `bus_*` tools in the session, plus its own `bus_spawn_worker`;
- streams the agent's inbox into the session as `[agent-bus]` messages, then acknowledges each one;
- adds the `/bus` command to manage credentials and a local server.

Server behaviour (spaces, topics, blackboards, permissions) is specified in [`server/spec/README.md`](../server/spec/README.md). Agent-facing usage guidance is in [`skills/agent-message-bus/SKILL.md`](../skills/agent-message-bus/SKILL.md).

Every tool's arguments, result shape and error codes are listed in [`server/TOOLS.md`](../server/TOOLS.md).

## Install

The repo root is an omp plugin that ships this extension and the skill:

```sh
omp plugin install github:till-muellers/agent-message-bus
omp plugin upgrade   # later, to update
```

## Commands

| Command | Effect |
| :--- | :--- |
| `/bus login <url> <token>` | Calls `bus_whoami` with the token. **Admin** token: stored as `adminToken`, nothing else starts; reply `Admin token stored. Run /bus promote <leader-name> to act as a leader.` A stored agent token is kept if the url is the same. **Leader/member** token: stored as `token`, the session's promoted leader token (if any) and its accepted member token (if any) are dropped, then tools are registered and the inbox stream starts; reply `Logged in as <kind> <name>`. A stored admin token is kept only if the url is the same, so `/bus promote` never sends one server's admin token to another. A trailing `/` on the url is stripped. |
| `/bus accept <url> <code>` | Redeems an invite code from `bus_invite_member` (single-use, valid for 10 minutes; case and dashes don't matter) with `POST <url>/v1/invites/accept`. The member token is stored for this omp session only (see [Session tokens](#session-tokens)); the session's promoted leader token and env credentials are dropped, then tools are registered and the inbox stream starts. If the invite carries a first prompt (`bus_invite_member`'s `task`), it is sent as a user message once the session has joined. Reply `Joined space <space> as member <name>.`, plus ` Not persisted: this session has no session file.` when the session has no file. A trailing `/` on the url is stripped. |
| `/bus spawn <space> <name> [task]` | Calls `bus_invite_member {space, name, task?}` with the active credentials and opens the new member as a separate omp session, `omp [<extension flags>] --cwd <this session's cwd> --bus-url <url> --bus-accept <code>`, in a new terminal tab (see [Worker spawn](#worker-spawn)). The rest of the line is the worker's first prompt; it travels with the invite on the server and comes back with the worker's accept, so it never appears on a command line. The code is never shown. Reply `Started worker <name> in space <space> (<launcher>).` Without credentials: `Not logged in to the agent bus. Run /bus login <url> <token>.` |
| `/bus promote <leader-name>` | Requires a stored admin token (otherwise `Run /bus login <url> <admin-token> first.`). Calls `bus_mint_leader` and restarts with the new leader token. The token is held in this session's memory only and never written to `credentials.json`; other and later sessions are unaffected. Reply `This session now acts as leader <name> (until it ends; run /bus promote again after a restart).` |
| `/bus` or `/bus status` | Calls `bus_whoami` with the active credentials (see precedence below); if there is no agent token, it uses the stored admin token. Prints `<url>: <kind> <name>; spaces: <name> (<role>), …`, then the extension version (`extension <version>`) and the installed omp plugin version (`installed plugin <version>`, `unknown` when it cannot be read), and warns when the installed plugin is older than the extension. Or `Not logged in.` with usage. |
| `/bus logout` | Deletes the credentials file and this session's accepted token file, drops the session's promoted leader token and forgets the env credentials read at load (no fallback remains). Stops the inbox stream and deactivates every `bus_*` tool; only `/bus login` or `/bus server start` re-activates them. Does not stop a server. Reply `Logged out; bus tools deactivated.` |
| `/bus server start` | Starts a local agent-message-bus (see [Local server](#local-server)), then stores its url and admin token like an admin `/bus login` and re-activates the tools. Reply `Agent message bus running at <url>; admin token stored. Run /bus promote <leader-name> to act as a leader.` |
| `/bus server stop` | Stops the local server, then runs `/bus logout`. |
| `/bus server status` | Prints the server's `status` output. |
| `/bus help` | Prints usage. |

Server errors are shown as `<code>: <message>`.

Example:

```text
/bus login http://localhost:8080 <admin-token>
/bus promote lead
/bus status
http://localhost:8080: leader lead; spaces: none
```

A worker can also join when its session starts: `omp --bus-url <url> --bus-accept <code>` redeems the code like `/bus accept`, shows the same reply and sends the invite's first prompt. Without `--bus-url`, the stored url is used; with neither, the session logs `agent-bus: --bus-accept needs --bus-url`. The flag is redeemed once per process, in top-level sessions only (subagents and later session switches ignore it). A failed redemption is logged as `agent-bus: <code>: <message>` and the session starts with its other credentials.

## Credentials

- File: `<PI_CODING_AGENT_DIR or ~/.omp/agent>/agent-message-bus/credentials.json`, shape `{url, adminToken?, token?}`. `/bus promote` never writes it.
- `OMP_AGENT_BUS_DIR` replaces the whole directory (the file is `$OMP_AGENT_BUS_DIR/credentials.json`).
- The file is written with mode `0600` and existing files are chmod-ed to `0600`. Windows ignores the mode; the user-profile ACL protects it.
- A corrupt file fails with `Credential file <path> is unreadable. Run /bus login to rewrite it.`
- **Environment:** `AGENT_BUS_URL`, `AGENT_BUS_TOKEN` and `AGENT_BUS_BIN` are read once when the extension loads and then removed from `process.env`, so child processes and subagents never inherit them.
- **One owner:** `lib/session/session-credentials.ts` holds the promoted, env and accepted credentials of the session; the callers name what happened (`promote`, `loggedIn`, `inviteAccepted`, `loggedOut`, `ended`) instead of writing the state. The precedence below is unchanged.
- **Precedence:** the session's promoted leader token (`/bus promote`) wins. Next, if `AGENT_BUS_URL` **and** `AGENT_BUS_TOKEN` were both set at load, they win (until `/bus logout`). Next comes the member token this session accepted (`/bus accept`). Otherwise the stored `token` is used. With only an admin token, the session has no tools and no inbox. Workers normally join with `/bus spawn` or `/bus accept`; the env override still hands a session a token directly (for example a member token redeemed with `POST /v1/invites/accept`) without touching the shared file. Each worker must be given its own variables explicitly:

  ```powershell
  $env:AGENT_BUS_URL = "http://localhost:8080"; $env:AGENT_BUS_TOKEN = "<member token>"; omp
  ```

### Session tokens

- `/bus accept` (and `--bus-accept`) writes `{url, token}` to `<agent-message-bus dir>/sessions/<session id>.json` (same directory rules and `0600` mode as `credentials.json`), where `<session id>` is the omp session file's name without `.jsonl`. `omp --resume` and `omp -c` reopen the same file, so a resumed worker keeps its identity without a new code.
- Subagents (whose session files live in `<parent session>/<id>.jsonl`) use their top-level session's file, so they act as the member their parent accepted.
- A session without a session file keeps the accepted token in memory until it ends.
- On every session change omp reports (`session_start`, `session_switch` for `/new`, `/resume` and `/fork`, `session_branch`), the extension re-keys the accepted token by the now-current session file and reconnects: the tools and the inbox stream follow the session's own identity. `/resume` of a worker's session restores its member token. `/new` starts without one (even without session files), so it falls back to the env or stored credentials. A branch (`session_branch`: `/branch`, `/btw`, a tree fork) and `/fork` always move the session onto a new session file; the extension copies the previous session's accepted token to the new file, so the branched worker stays the same member.
- A corrupt file fails with `Session credential file <path> is unreadable. Run /bus accept again.`
- `/bus logout` and `/bus login` with an agent token delete the session's file.

## Tools

At session start (or on `/bus login` or `/bus promote`), the extension fetches `GET /v1/tools` once per session. It registers each tool under its server name, converting the JSON Schema to zod via `pi.zod`. A tool that cannot be registered (for example an unsupported schema) is logged as `agent-bus: tool <name> not registered: …`; the other tools still register. Each call uses the credentials that are active **at call time**, so `/bus promote` changes identity without re-registering. A tool result is the server's `result` as pretty-printed JSON. A server error is thrown as `<code>: <message>`. The extension's tools follow the resolved credentials: whenever they are re-resolved (session start, switch, branch, login, promote, accept), its `bus_*` tools are switched on if the session has agent credentials and off otherwise; an admin token alone (`/bus login <url> <admin-token>`, `/bus server start`) switches none on. After `/bus logout` every `bus_*` tool is deactivated until agent credentials resolve again. If the tool fetch fails, the inbox stream is not started either; the error is logged as `agent-bus: …` (or shown as the command reply), and the next `/bus login` retries.

**Subagents** (sessions with `agent.kind === "sub"`) don't get `bus_inbox_read` or `bus_inbox_ack` and run no inbox stream: the parent session owns the inbox.

**`bus_spawn_worker`** `{space, name, task?, cwd?}` is registered by the extension itself, inactive; it is switched on and off with the other bus tools, so only while the session has agent credentials. It does what `/bus spawn` does; `cwd` is resolved against this session's working directory and defaults to it. Leaders of the space only, since it calls `bus_invite_member`. It returns `{name, space, launcher}`; the invite code is never exposed. Aborting the tool call kills a launcher CLI that is still running. Errors are thrown as `<code>: <message>` (bus errors) or `<launcher>: <message>` (launch failures).

## Worker spawn

`/bus spawn` and `bus_spawn_worker` open the worker with the first launcher whose environment is detected, checked in this order (the innermost surface wins: an Orca terminal running tmux or zellij opens a tmux window or zellij tab):

| Launcher | Detected when | Runs |
| :--- | :--- | :--- |
| `tmux` | `$TMUX` is set | `tmux new-window -n <name> -- omp …` |
| `zellij` | `$ZELLIJ` is set | `zellij action new-tab --name <name> -- omp …` (needs zellij ≥ 0.44) |
| `orca` | `TERM_PROGRAM=Orca` | `orca terminal create --title <name> --command '<omp …>' --json` (`$ORCA_CLI_COMMAND` replaces `orca`; on Windows with `--shell powershell.exe`) |
| `wezterm` | `$WEZTERM_PANE` is set | `wezterm cli spawn -- omp …` |
| `kitty` | `$KITTY_WINDOW_ID` is set | `kitty @ launch --type=tab --tab-title <name> -- omp …` (needs `allow_remote_control` in kitty.conf) |
| `windows-terminal` | Windows and `$WT_SESSION` is set | `wt.exe -w 0 new-tab --title <name> -- omp …` |
| `macos-terminal` | macOS | Terminal.app via `osascript` |
| `linux-terminal` | Linux with `$DISPLAY` or `$WAYLAND_DISPLAY` set, and one of `x-terminal-emulator`, `gnome-terminal`, `konsole`, `xterm` on `PATH` | the first of them found, started detached |
| `windows-console` | Windows | a new console window via PowerShell `Start-Process` |

Detection does all the environment probing (variables, platform, `PATH`), so with no match the spawn fails with `no terminal launcher found for this environment` before anything is invited. A launcher that fails (non-zero exit) reports `<launcher>: <command> exited with <code>: <output>`; a launcher CLI that has not returned after 15 s is killed and reports `<launcher>: run <command>: timed out after 15000 ms`. The invite code (and the first prompt with it) then expires unused after 10 minutes.

The worker's omp must know `--bus-url` and `--bus-accept`, which exist only while this extension is loaded, and omp exits on unknown flags. A plugin installed with `omp plugin install` is loaded everywhere. If the leader loaded it from the command line instead, the leader's `--plugin-dir <p>`, `--extension <p>`, `-e <p>` and `--trusted-extension <p>` flags (and their `--flag=<p>` forms) are forwarded to the worker right after `omp`. The new tab may start in another directory than the leader, so relative `--plugin-dir` and `-e`/`--extension` values are resolved against the leader's working directory first; omp treats every `-e` value as a path, so a bare name such as `index.ts` is resolved too. Values starting with `~`, absolute values and `--trusted-extension` values (which omp requires to be absolute) are forwarded unchanged.

Don't load two copies of this extension in one session, such as an installed plugin plus `-e <repo>/extension/index.ts`. Both copies register the same `bus_*` tools and `/bus` command. `AGENT_BUS_URL`/`AGENT_BUS_TOKEN` are read once at load and deleted from the environment, so only the first copy to load sees them. For development against a working tree, use a stored login in a separate `OMP_AGENT_BUS_DIR`.

Before it invites anybody, the spawn checks the plugin version the worker would load: the forwarded extension's own `package.json` decides, else the installed omp plugin (`omp plugin list --json`). A worker whose plugin is older than this one is refused with `the worker would load omp plugin <version>, which is older than the protocol <version>; run omp plugin upgrade @till-muellers/omp-agent-message-bus`; no invite code is spent and no terminal opens. An unknown version proceeds.

## Inbox delivery

Only top-level sessions stream the inbox; subagents don't.


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

The server allows one live inbox stream per agent, so each agent identity belongs to exactly one session. When another session already streams this agent's inbox, the server answers `409 conflict`. The extension then logs once, `agent-bus: another session is already connected as <name>; inbox not delivered here` (the name comes from `bus_whoami`; if that fails, `as this agent`), and keeps retrying with the normal backoff. The inbox is delivered here once the other stream ends, for example when its session exits or crashes (the server notices a dead connection at its next heartbeat, within about 20 s). Once this session's stream has opened, a later refusal logs again. Tool calls are not restricted. Give every agent its own identity: `/bus promote` is session-scoped, and each worker joins with its own invite code (`/bus spawn`, `/bus accept`) or its own member token via env.

## Local server

`/bus server start|stop|status` drives the server's own CLI (`start`, `stop`, `status --json`, `admin-token`). The binary is resolved in this order:

1. `AGENT_BUS_BIN` (read once at load);
2. `agent-message-bus` (`.exe` on Windows) on `PATH`;
3. a download of the latest GitHub release of `till-muellers/agent-message-bus` for this platform, verified against the release's `SHA256SUMS` and cached under `<PI_CODING_AGENT_DIR or ~/.omp/agent>/agent-message-bus/bin/<version>/`.

Server behaviour (single instance per data dir, admin token file) is described in [`server/README.md`](../server/README.md).
