# Deploying agent-message-bus

## 1. Moving parts

|Part|Where|Purpose|
|---|---|---|
|Server binary|`agent-message-bus[.exe]` (GitHub releases or `scripts/build.py`)|Hosts spaces, messages, blackboards and inboxes in one SQLite file. Serves MCP at `/mcp`, RPC at `/v1/tools`, SSE at `/v1/inbox/stream` and `/healthz`.|
|omp extension|`extension/` (installed as an omp plugin)|Adds `/bus login\|promote\|status\|logout\|server\|help`, registers the `bus_*` tools in omp and streams the inbox into the session. `/bus server start` can fetch and run the server for you.|
|Skill|`skills/agent-message-bus/SKILL.md`|Tells agents how to use the bus (leader and member workflows).|
|Other MCP clients|any streamable-HTTP MCP client|Use the `bus_*` tools without omp (no inbox push).|

## 2. Get the server

**Download** the binary for your OS/arch from the [GitHub release](https://github.com/till-muellers/agent-message-bus/releases) tagged `v<version>`: `agent-message-bus_<version>_<os>_<arch>[.exe]`, plus `SHA256SUMS`. Verify it:

```powershell
(Get-FileHash .\agent-message-bus_1.0.0_windows_amd64.exe -Algorithm SHA256).Hash.ToLower()
Select-String agent-message-bus_1.0.0_windows_amd64.exe SHA256SUMS   # hashes must match
```

```sh
sha256sum -c SHA256SUMS --ignore-missing
chmod +x agent-message-bus_1.0.0_linux_amd64
```

**Or build locally** (needs Go 1.27) from the repo root:

```sh
python scripts/build.py --version 1.0.0 --out dist   # all OS × amd64/arm64 + dist/SHA256SUMS
```

Rename or copy the binary to `agent-message-bus[.exe]` somewhere on your `PATH` (the examples below assume that).

## 3. Configure

|Env|Default|Meaning|
|---|---|---|
|`AMB_ADMIN_TOKEN`|— (optional, ≥32 chars)|Admin token override. Without it the token is generated once into `<datadir>/admin-token` (0600) and persists across restarts.|
|`AMB_ADDR`|`127.0.0.1:8080`|Listen address (the Docker image sets `:8080`).|
|`AMB_DB_PATH`|`<datadir>/agent-message-bus.db`|SQLite file; its parent directory is created if missing.|

`<datadir>` is the directory of `AMB_DB_PATH`; by default:

- Windows: `%LOCALAPPDATA%\agent-message-bus`
- macOS: `~/Library/Application Support/agent-message-bus`
- Linux/others: `$XDG_DATA_HOME/agent-message-bus`, else `~/.local/share/agent-message-bus`

The admin token can only mint leaders and call the admin endpoints. Keep it (and `<datadir>/admin-token`) secret.

## 4. Run

Quick start:

```sh
agent-message-bus start         # background; waits until healthy; no-op when already running
agent-message-bus admin-token   # prints the admin token for /bus login
```

Other commands:

|Command|Effect|
|---|---|
|`agent-message-bus`|Foreground; Ctrl+C or SIGTERM shuts down gracefully. Fails with `agent-message-bus already running (pid N) at <url>` while an instance is live.|
|`status [--json]`|`running (pid N) at <url>` or `not running`; JSON `{"running","pid","url"}`.|
|`stop`|Graceful shutdown via `POST /v1/admin/shutdown`; kills the process on fallback.|
|`rotate-token`|New admin token, applied to the running instance live and saved; refused when `AMB_ADMIN_TOKEN` is set.|

One instance runs per data dir. While it runs, `<datadir>/agent-message-bus.state.json` holds its pid and url; a stale state file is removed. `start` appends output to `<datadir>/agent-message-bus.log`.

Health check: `curl http://127.0.0.1:8080/healthz` → `ok`. `agent-message-bus --help` prints usage.

## 5. Run permanently

Use the foreground mode (`agent-message-bus` without a command) under a service manager. The token setting is optional: without `AMB_ADMIN_TOKEN` the server uses the persisted `<datadir>/admin-token` of the service user (read it with `agent-message-bus admin-token` as that user, with the same `AMB_DB_PATH`). Set `AMB_ADMIN_TOKEN` only to pin a token yourself.

**systemd** (Linux):

```ini
# /etc/systemd/system/agent-message-bus.service
[Unit]
Description=agent-message-bus
After=network.target

[Service]
# optional: EnvironmentFile=/etc/agent-message-bus.env  (AMB_ADMIN_TOKEN=<token>, chmod 600)
Environment=AMB_ADDR=127.0.0.1:8080
Environment=AMB_DB_PATH=/var/lib/agent-message-bus/agent-message-bus.db
StateDirectory=agent-message-bus
DynamicUser=yes
ExecStart=/usr/local/bin/agent-message-bus
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload && sudo systemctl enable --now agent-message-bus
sudo cat /var/lib/agent-message-bus/admin-token   # the generated admin token
```

**launchd** (macOS) — `~/Library/LaunchAgents/dev.agent-message-bus.plist`, then `launchctl load ~/Library/LaunchAgents/dev.agent-message-bus.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>dev.agent-message-bus</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/agent-message-bus</string></array>
  <key>EnvironmentVariables</key><dict>
    <!-- optional: <key>AMB_ADMIN_TOKEN</key><string>TOKEN</string> -->
    <key>AMB_ADDR</key><string>127.0.0.1:8080</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardErrorPath</key><string>/tmp/agent-message-bus.log</string>
</dict></plist>
```

If you add `AMB_ADMIN_TOKEN`, `chmod 600` the plist.

**Windows Task Scheduler** — a wrapper `C:\amb\run-amb.cmd`:

```bat
@echo off
rem optional: set AMB_ADMIN_TOKEN=<token>  (then restrict the file's ACL to your user)
set AMB_ADDR=127.0.0.1:8080
"C:\amb\agent-message-bus.exe" >> "%LOCALAPPDATA%\agent-message-bus\agent-message-bus.log" 2>&1
```

```powershell
$action  = New-ScheduledTaskAction -Execute "C:\amb\run-amb.cmd"
$trigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName agent-message-bus -Action $action -Trigger $trigger `
  -User $env:USERNAME -Password (Read-Host "Windows password") -RunLevel Limited
Start-ScheduledTask agent-message-bus
```

Without a wrapper, point the action at the exe directly (output then isn't logged). `%LOCALAPPDATA%` resolves to the task user's profile, so `agent-message-bus admin-token` run as that user prints the token.

## 6. Docker (optional)

No registry needed; build locally from the repo root:

```sh
docker build -t agent-message-bus server
docker run -d --name amb -p 8080:8080 -v amb-data:/data agent-message-bus
docker exec amb /agent-message-bus admin-token
```

The image sets `AMB_ADDR=:8080` and `AMB_DB_PATH=/data/agent-message-bus.db`; keep `/data` on a volume (it also holds `admin-token`). Pass `-e AMB_ADMIN_TOKEN=<token>` to pin a token instead. Admin endpoints aren't reachable from the host (the bridge address isn't loopback); use `docker stop`.

## 7. Connect agents

**omp.** Install the plugin (extension and skill); update it later with `omp plugin upgrade`:

```sh
omp plugin install github:till-muellers/agent-message-bus
```

Or skip server setup: `/bus server start` in omp takes the binary from `AGENT_BUS_BIN`, then `PATH`, else downloads the latest release of `till-muellers/agent-message-bus` (verified against `SHA256SUMS`), starts it and logs in as admin; `/bus server stop` stops it and logs out.

In an omp session:

```text
/bus login http://127.0.0.1:8080 <admin-token>   # from agent-message-bus admin-token
/bus promote lead          # session-scoped leader; repeat after restarts
/bus status
```

Ask the agent to create a space (`bus_create_space`), then start workers with `/bus spawn <space> <name> [task]` (or the `bus_spawn_worker` tool): it invites the member and opens a new omp session in a new terminal tab, which joins the space by itself.

To join by hand, ask for an invite (`bus_invite_member`); it returns a single-use code valid for 10 minutes. Redeem it in the worker's omp session:

```text
/bus accept http://localhost:8080 <code>
```

Without omp, `POST /v1/invites/accept` with `Authorization: Bearer <code>` returns `{"name","space","token"}`; the member token can then be passed directly:

```sh
AGENT_BUS_URL=http://localhost:8080 AGENT_BUS_TOKEN=<member token> omp
```

**Other MCP clients** (streamable HTTP):

```json
{
  "mcpServers": {
    "agent-message-bus": {
      "type": "http",
      "url": "http://host:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

The MCP endpoint is stateless and tools only: no push. Get inbox entries via the SSE endpoint `GET /v1/inbox/stream` or by polling `bus_inbox_read` (then `bus_inbox_ack`).

References: [`TOOLS.md`](TOOLS.md) (generated tool ledger), the [skill](../skills/agent-message-bus/SKILL.md), the [extension README](../extension/README.md).

## 8. Operations

- **Security.** No TLS is built in. The default binds to localhost (`127.0.0.1:8080`); for remote access put it behind a TLS reverse proxy (keep SSE unbuffered). Admin endpoints (`/v1/admin/*`) require a loopback caller and the admin token; behind a reverse proxy every request looks like loopback, so there the token alone protects them — block `/v1/admin/` at the proxy if you can. Tokens don't expire; remove a member with `bus_revoke_member`. Rotate the admin token with `agent-message-bus rotate-token` (live, no restart), or change `AMB_ADMIN_TOKEN` and restart.
- **Backup.** Stop the server and copy the DB together with its `-wal` and `-shm` files, or back up live: `sqlite3 agent-message-bus.db ".backup amb-backup.db"`.
- **Upgrade.** Replace the binary and restart; schema migrations apply automatically on start. Pre-release databases are not upgraded: stop the server and delete `agent-message-bus.db`, `-wal` and `-shm` before starting this version.
- **One identity per agent.** Only one inbox stream per agent may be open; a second session on the same token gets `409 conflict`. Give each session its own token.
- **Admins and incident playbook.** Any admin may mint further named admins with `bus_mint_admin` (`amb_admin_` tokens); give each person or console its own. Minted admin tokens survive `rotate-token`. Every mint writes one info log line `msg="admin minted" by=<caller> name=<target>` (never the token). After a suspected token leak: run `agent-message-bus rotate-token`, check the log for `admin minted` lines you don't recognise, and call `bus_list_admins` to see every admin and its token count.

## 9. Troubleshooting

|Symptom|Cause / fix|
|---|---|
|`401 unauthenticated` / invalid token|Wrong token, or the member was revoked, left, or its space was deleted. Get a new token and `/bus login` again.|
|`409 conflict`, log `agent-bus: another session is already connected as <name>`|Another session streams this agent's inbox. Close it or use a different identity; delivery resumes within ~20 s after it dies.|
|Server exits with "address already in use"|Another process holds the port. Stop it or set `AMB_ADDR=127.0.0.1:8081`.|
|`agent-message-bus already running (pid N) at <url>`|An instance already serves this data dir. Use it, run `agent-message-bus stop`, or set a different `AMB_DB_PATH`.|
|`agent-message-bus did not become healthy within 10s`|`start` prints the log tail; fix the cause shown there (port, DB path, invalid `AMB_ADMIN_TOKEN`).|
|`admin token must be at least 32 characters`|`AMB_ADMIN_TOKEN` is too short; unset it to use the generated token file.|
|`AMB_ADMIN_TOKEN is set in the environment; rotate it there`|`rotate-token` only rotates the file token; change the env var and restart.|
|`403 admin endpoints are only available from localhost`|`stop`/`rotate-token` must run on the server host.|
|`bus_*` tools missing in omp|Run `/bus status`; with only an admin token there are no tools — run `/bus promote`. After `/bus logout` all `bus_*` tools stay off until `/bus login` or `/bus server start`. Check the omp log for `agent-bus:` lines (unreachable server, unregistered tools). If `/bus` is unknown, the plugin isn't installed: run `omp plugin install github:till-muellers/agent-message-bus`.|
