# Deploying agent-message-bus

## 1. Moving parts

|Part|Where|Purpose|
|---|---|---|
|Server binary|`agent-message-bus[.exe]` (GitHub Releases or `scripts/build_amb.py`)|Hosts spaces, messages, blackboards and inboxes in one SQLite file. Serves MCP at `/mcp`, RPC at `/v1/tools`, SSE at `/v1/inbox/stream` and `/healthz`.|
|omp extension|`extensions/omp/agent-message-bus`|Adds `/bus login\|promote\|status\|logout\|help`, registers the `bus_*` tools in omp and streams the inbox into the session.|
|Skill|`skills/agent-message-bus/SKILL.md`|Tells agents how to use the bus (leader and member workflows).|
|Other MCP clients|any streamable-HTTP MCP client|Use the `bus_*` tools without omp (no inbox push).|

## 2. Get the server

**Download** the binary for your OS/arch from the GitHub Release tagged `amb-v<version>`: `agent-message-bus_<version>_<os>_<arch>[.exe]`, plus `SHA256SUMS`. Verify it:

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
python scripts/build_amb.py --version 1.0.0 --out dist   # all OS × amd64/arm64 + dist/SHA256SUMS
```

Rename or copy the binary to `agent-message-bus[.exe]` somewhere on your `PATH` (the examples below assume that).

## 3. Configure

|Env|Default|Meaning|
|---|---|---|
|`AMB_ADMIN_TOKEN`|— (required, ≥32 chars)|Admin token; it can only mint leader tokens.|
|`AMB_ADDR`|`:8080`|Listen address.|
|`AMB_DB_PATH`|`<datadir>/agent-message-bus.db`|SQLite file; its parent directory is created if missing.|

`<datadir>`:

- Windows: `%LOCALAPPDATA%\agent-message-bus`
- macOS: `~/Library/Application Support/agent-message-bus`
- Linux/others: `$XDG_DATA_HOME/agent-message-bus`, else `~/.local/share/agent-message-bus`

Generate an admin token:

```powershell
[guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
```

```sh
openssl rand -hex 32
```

Keep it secret: it can mint leaders.

## 4. Run

Foreground (Ctrl+C or SIGTERM shuts down gracefully):

```powershell
$env:AMB_ADMIN_TOKEN = "<token>"; agent-message-bus
```

```sh
AMB_ADMIN_TOKEN=<token> agent-message-bus
```

Background, detached from the terminal:

```sh
agent-message-bus --detached
# agent-message-bus started (pid 1234); log: <dir>/agent-message-bus.log; pid file: <dir>/agent-message-bus.pid
```

`<dir>` is the directory of `AMB_DB_PATH`. The log is appended to. Stop it with the OS:

```sh
kill $(cat ~/.local/share/agent-message-bus/agent-message-bus.pid)
```

```powershell
Stop-Process -Id (Get-Content "$env:LOCALAPPDATA\agent-message-bus\agent-message-bus.pid")
```

Health check: `curl http://localhost:8080/healthz` → `ok`. `agent-message-bus --help` prints usage.

## 5. Run permanently

Use the foreground mode under a service manager (not `--detached`).

**systemd** (Linux) — `/etc/agent-message-bus.env` holds `AMB_ADMIN_TOKEN=<token>` (`chmod 600`):

```ini
# /etc/systemd/system/agent-message-bus.service
[Unit]
Description=agent-message-bus
After=network.target

[Service]
EnvironmentFile=/etc/agent-message-bus.env
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
```

**launchd** (macOS) — `~/Library/LaunchAgents/dev.agent-message-bus.plist`, then `launchctl load ~/Library/LaunchAgents/dev.agent-message-bus.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>dev.agent-message-bus</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/agent-message-bus</string></array>
  <key>EnvironmentVariables</key><dict>
    <key>AMB_ADMIN_TOKEN</key><string>TOKEN</string>
    <key>AMB_ADDR</key><string>127.0.0.1:8080</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardErrorPath</key><string>/tmp/agent-message-bus.log</string>
</dict></plist>
```

`chmod 600` the plist since it contains the token.

**Windows Task Scheduler** — a wrapper `C:\amb\run-amb.cmd` (restrict its ACL to your user):

```bat
@echo off
set AMB_ADMIN_TOKEN=<token>
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

Alternatively set the token as a user env var (`[Environment]::SetEnvironmentVariable('AMB_ADMIN_TOKEN','<token>','User')`) and point the action at the exe directly. `%LOCALAPPDATA%` resolves to the task user's profile.

## 6. Docker (optional)

No registry needed; build locally:

```sh
docker build -t agent-message-bus apps/agent-message-bus
docker run -d --name amb -p 8080:8080 -e AMB_ADMIN_TOKEN=<token> -v amb-data:/data agent-message-bus
```

The image sets `AMB_DB_PATH=/data/agent-message-bus.db`; keep `/data` on a volume.

## 7. Connect agents

**omp.** Deploy the extension and skill from the repo root:

```sh
python scripts/seed.py --dry-run   # review
python scripts/seed.py
```

In an omp session:

```text
/bus login http://localhost:8080 <admin-token>
/bus promote lead          # session-scoped leader; repeat after restarts
/bus status
```

Ask the agent to create a space (`bus_create_space`) and invite members (`bus_invite_member`); each invite returns a member token. Start a worker session per member on its own token:

```powershell
$env:AGENT_BUS_URL = "http://localhost:8080"; $env:AGENT_BUS_TOKEN = "<member token>"; omp
```

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

References: [`TOOLS.md`](TOOLS.md) (generated tool ledger), the [skill](../../skills/agent-message-bus/SKILL.md), the [extension README](../../extensions/omp/agent-message-bus/README.md).

## 8. Operations

- **Security.** No TLS is built in. Bind to localhost (`AMB_ADDR=127.0.0.1:8080`) or put it behind a TLS reverse proxy (keep SSE unbuffered). Tokens don't expire; remove a member with `bus_revoke_member`. Rotating the admin token only needs a restart with the new value.
- **Backup.** Stop the server and copy the DB together with its `-wal` and `-shm` files, or back up live: `sqlite3 agent-message-bus.db ".backup amb-backup.db"`.
- **Upgrade.** Replace the binary and restart; schema migrations apply automatically on start.
- **One identity per agent.** Only one inbox stream per agent may be open; a second session on the same token gets `409 conflict`. Give each session its own token.

## 9. Troubleshooting

|Symptom|Cause / fix|
|---|---|
|`401 unauthenticated` / invalid token|Wrong token, or the member was revoked, left, or its space was deleted. Get a new token and `/bus login` again.|
|`409 conflict`, log `agent-bus: another session is already connected as <name>`|Another session streams this agent's inbox. Close it or use a different identity; delivery resumes within ~20 s after it dies.|
|Server exits with "address already in use"|Another process holds the port. Stop it or set `AMB_ADDR=127.0.0.1:8081`.|
|`AMB_ADMIN_TOKEN is required`|Set the env var (≥32 chars) in the shell, service unit, plist or wrapper.|
|`bus_*` tools missing in omp|Run `/bus status`; with only an admin token there are no tools — run `/bus promote`. Check the omp log for `agent-bus:` lines (unreachable server, unregistered tools). Re-run `scripts/seed.py` if `/bus` is unknown.|
