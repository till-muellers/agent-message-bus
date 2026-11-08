# Deploying agent-message-bus

## 1. Moving parts

|Part|Where|Purpose|
|---|---|---|
|Server binary|`agent-message-bus[.exe]` (GitHub releases or `scripts/build.py`)|Hosts spaces, messages, blackboards and inboxes in one SQLite file. Serves MCP at `/mcp`, RPC at `/v1/tools`, SSE at `/v1/inbox/stream` and `/healthz`.|
|omp extension|`extension/` (installed as an omp plugin)|Adds `/bus login\|promote\|accept\|spawn\|status\|logout\|server\|help`, registers the `bus_*` tools in omp and streams the inbox into the session. `/bus server start` can fetch and run the server for you.|
|Agent guide|`server/internal/guide/guide.md`, embedded in the binary|Tells agents how to use the bus (leader and member workflows). Served as `GET /v1/guide` and as the MCP `instructions`; the omp extension puts it into every model request.|
|Other MCP clients|any streamable-HTTP MCP client|Use the `bus_*` tools without omp (no inbox push).|

## 2. Get the server

**Download** the binary for your OS/arch from the [GitHub release](https://github.com/till-muellers/agent-message-bus/releases) tagged `v<version>`: `agent-message-bus_<version>_<os>_<arch>[.exe]`, plus `SHA256SUMS`. Verify it:

```powershell
(Get-FileHash .\agent-message-bus_X.Y.Z_windows_amd64.exe -Algorithm SHA256).Hash.ToLower()
Select-String agent-message-bus_X.Y.Z_windows_amd64.exe SHA256SUMS   # hashes must match
```

```sh
sha256sum -c SHA256SUMS --ignore-missing
chmod +x agent-message-bus_X.Y.Z_linux_amd64
```

**Or build locally** (needs Go 1.27) from the repo root:

```sh
python scripts/build.py --version X.Y.Z --out dist   # windows, linux × amd64/arm64 + dist/SHA256SUMS
```

Releases ship Windows and Linux (amd64, arm64) binaries. macOS is not supported; build from source with `cd server && go build -o agent-message-bus .` (or `scripts/build.py`).

Rename or copy the binary to `agent-message-bus[.exe]` somewhere on your `PATH` (the examples below assume that).

## 3. Configure

|Env|Default|Meaning|
|---|---|---|
|`AMB_ADMIN_TOKEN`|— (optional, 32..512 visible ASCII chars, trimmed)|Admin token override. Without it the token is generated once into `<datadir>/admin-token` (0600) and persists across restarts.|
|`AMB_ADDR`|`127.0.0.1:8080`|API listen address (the Docker image sets `:8080`).|
|`AMB_WEB_ADDR`|`127.0.0.1:8081`|Web GUI listen address, any `host:port`; empty or `off` disables the GUI (the Docker image sets `off`). It must differ from `AMB_ADDR`; a GUI address that can't be bound stops the start with an error naming `AMB_WEB_ADDR`, also one that overlaps `AMB_ADDR` on the same port (a wildcard such as `:8080` or `0.0.0.0:8080` next to `127.0.0.1:8080`, in either order) on Windows (`SO_EXCLUSIVEADDRUSE`) and Linux; macOS still allows that overlap, so keep the ports apart there. The GUI is served only here (`http://127.0.0.1:8081/web/`); the API listener has no `/web` route.|
|`AMB_DB_PATH`|`<datadir>/agent-message-bus.db`|SQLite file; its parent directory is created if missing.|
|`AMB_REQUIRE_EXPLICIT`|unset (guard off)|Opt-in guard: with `1`, every command refuses to run unless `AMB_ADDR`, `AMB_WEB_ADDR`, `AMB_DB_PATH` and `AMB_ADMIN_TOKEN` are all given, so a shell that sets it can never fall back to the default data dir or ports. For development shells and coding agents; do not set it on a service.|

`<datadir>` is the directory of `AMB_DB_PATH`; by default:

- Windows: `%LOCALAPPDATA%\agent-message-bus`
- macOS: `~/Library/Application Support/agent-message-bus`
- Linux/others: `$XDG_DATA_HOME/agent-message-bus`, else `~/.local/share/agent-message-bus`

The admin token mints leaders, mints and revokes admins, calls the admin endpoints, leads every space without joining it and may delete any space. Keep it (and `<datadir>/admin-token`) secret.

The web GUI gets its own address so you decide its exposure separately from the API: the default keeps it on loopback, `0.0.0.0:8081` (or any other host) exposes it, `off` turns it off. Sign in there with any token (the admin token for the admin view); the browser keeps only an opaque session cookie, never the token.

## 4. Run

Quick start:

```sh
agent-message-bus start         # background; waits until healthy; no-op when already running
agent-message-bus admin-token   # prints the admin token for /bus login
```

Other commands:

|Command|Effect|
|---|---|
|`agent-message-bus`|Foreground; Ctrl+C or SIGTERM shuts down gracefully. Fails with `agent-message-bus already running (pid N) at <url>` (or `… (state pending)` while that instance has not published its url yet) while an instance runs on this data dir.|
|`status [--json]`|`running (pid N) at <url>`, `running (state pending)` or `not running`; JSON `{"running","pid","url"}` (pid and url only once the instance is confirmed at its url).|
|`stop`|Graceful shutdown via `POST /v1/admin/shutdown`; kills the process when it still holds the data dir lock after 10 s and the state file still names it. A server that answers the request with 401 (token not accepted) is not stuck: `stop` fails at once with that answer, kills nothing and leaves the instance running (the admin token it uses is not the instance's). Acts only on an instance confirmed at its url; one in `state pending` for 10 s is an error, never a target (`start` and `rotate-token` wait for it the same way).|
|`rotate-token`|New admin token, applied to the running instance live and saved; refused when `AMB_ADMIN_TOKEN` is set. Runs on one data dir take turns (lock file `<datadir>/admin-token.lock`), so the saved token is always the one the instance accepts.|

One instance runs per data dir. The server holds the OS lock `<datadir>/agent-message-bus.lock` from before it opens the database until it exits, so a second server on the same data dir fails even when both start at the same moment; the OS frees the lock when the process ends, also after a crash or kill. While it runs, `<datadir>/agent-message-bus.state.json` holds its pid, url and instance id, which `/healthz` returns in the `Agent-Bus-Instance` header. The commands trust the lock: with the lock free, a leftover state file (also an empty or corrupt one) is stale and removed; with the lock held, the state file counts only once its url answers with its instance id (until then the instance is `state pending`), so a process of another data dir on the same url is never reported, shut down or killed. `start` appends output to `<datadir>/agent-message-bus.log`. A second server never waits for the instance that holds the lock: it fails at once (a start only waits out another command's brief probe of the lock), so once a losing server has met the lock, stopping the instance a racing start reported leaves nothing serving the data dir.

Health check: `curl http://127.0.0.1:8080/healthz` → `ok`. Web GUI: open `http://127.0.0.1:8081/web/`. `agent-message-bus --help` prints usage.

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

No registry needed; build locally from the repo root. The image embeds the web GUI from `server/internal/web/dist/`, so build the GUI first (needs [bun](https://bun.sh)); without that step the image serves the fallback page "The web GUI was not built" at `/web/`:

```sh
bun run build:gui                 # writes server/internal/web/dist/
docker build -t agent-message-bus server
docker run -d --name amb -p 8080:8080 -v amb-data:/data agent-message-bus
docker exec amb /agent-message-bus admin-token
```

The image sets `AMB_ADDR=:8080`, `AMB_WEB_ADDR=off` and `AMB_DB_PATH=/data/agent-message-bus.db`; keep `/data` on a volume (it also holds `admin-token`). Pass `-e AMB_ADMIN_TOKEN=<token>` to pin a token instead. Admin endpoints aren't reachable from the host (the bridge address isn't loopback); use `docker stop`.

To publish the web GUI, set its address explicitly and publish the port (the image only `EXPOSE`s 8080; the GUI is off unless you do this):

```sh
docker run -d --name amb -p 8080:8080 -p 127.0.0.1:8081:8081 -e AMB_WEB_ADDR=:8081 -v amb-data:/data agent-message-bus
```

## 7. Connect agents

**omp.** Install the plugin (the extension); update it later with `omp plugin upgrade @till-muellers/omp-agent-message-bus`:

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

References: [`TOOLS.md`](TOOLS.md) (generated tool ledger), the [agent guide](internal/guide/guide.md), the [extension README](../extension/README.md).

## 8. Operations

- **Security.** No TLS is built in. The defaults bind to localhost (API `127.0.0.1:8080`, web GUI `127.0.0.1:8081`); for remote access put them behind a TLS reverse proxy (keep SSE unbuffered). Admin endpoints (`/v1/admin/*`) require a loopback caller and the admin token; behind a reverse proxy every request looks like loopback, so there the token alone protects them — block `/v1/admin/` at the proxy if you can. Tokens don't expire; remove a member with `bus_revoke_member`. Rotate the admin token with `agent-message-bus rotate-token` (live, no restart), or change `AMB_ADMIN_TOKEN` and restart.
- **Host header behind a proxy.** The server never checks the `Host` header, so the proxy may forward the public host (the default of Caddy and of nginx with `proxy_set_header Host $host`) or rewrite it: `/mcp`, `/v1/*` and `/healthz` answer the same either way, also when the proxy connects over loopback. Authentication is the bearer token alone. The MCP SDK's DNS-rebinding guard, which answered a non-`localhost` `Host` on a loopback connection with a plain-text 403 `invalid Host header`, is switched off: a rebinding page can't present a token (the browser never adds the `Authorization` header by itself, and the API listener sends no CORS headers). No setting is needed.
- **Web GUI behind a proxy.** Expose the GUI listener (`AMB_WEB_ADDR`) as its own site or path prefix `/web/` and keep it on HTTPS: the server marks the session cookie `Secure` only when it terminates TLS itself, which it never does, so a TLS proxy is what keeps the cookie off plain HTTP. The sign-in limit (5 failures per IP and minute) counts the proxy's address when every request comes from the proxy, so one attacker can lock out everyone behind it for a minute; restrict who reaches the GUI at the proxy (VPN, SSO, IP allow list) or use an SSH tunnel (`ssh -L 8081:127.0.0.1:8081 host`). Sessions are in memory and last at most 8 hours; a restart signs everybody out. The firewall now has two ports to consider: 8080 (API) and 8081 (GUI).
- **Long calls behind a proxy.** `bus_wait` may block for up to 300 seconds. The server itself sets no read, write or idle timeout (only the inbox stream bounds each frame write to 30 seconds); a reverse proxy in front must allow at least 310 seconds on `/v1/tools/*` and `/mcp` (and keep `/v1/inbox/stream` unbuffered).
- **Backup.** Stop the server and copy the DB together with its `-wal` and `-shm` files, or back up live: `sqlite3 agent-message-bus.db ".backup amb-backup.db"`.
- **Upgrade.** Stop the server with the binary that started it, replace the binary and start again; schema migrations apply automatically on start. Stop it before you run any command of the new binary: the commands find a running instance by its data dir lock, which older binaries don't take, so the new binary treats the old instance's state file as stale and removes it, and then neither binary can stop that instance. Pre-release databases are not upgraded: stop the server and delete `agent-message-bus.db`, `-wal` and `-shm` before starting this version.
- **Downgrade.** Back up before every upgrade: migrations only go forward. To go back, stop the server and restore the backup taken before the upgrade, then start the older binary. Without a backup, run the newer migrations' `.down.sql` files newest first by hand, each with foreign keys off so a table rebuild doesn't cascade-delete child rows: `sqlite3 agent-message-bus.db` then `PRAGMA foreign_keys=OFF; BEGIN; .read <down file>` followed by `PRAGMA foreign_key_check; DELETE FROM schema_migrations WHERE version = <N>; COMMIT;` (commit only when the check prints nothing). This version refuses a database migrated by a newer one (`database schema version N is newer than this binary (latest M)`), but v0.2.0 and v0.3.0 cannot detect it: started on a newer schema they run anyway and corrupt data (v0.2.0 stores a new invite with id `''` and then fails every further `bus_invite_member` with `UNIQUE constraint failed: invites.id` until that invite is used or expires).
- **One identity per agent.** Only one inbox stream per agent may be open; a second session on the same token gets `409 conflict`. Give each session its own token.
- **Admins and incident playbook.** Any admin may mint further named admins with `bus_mint_admin` (`amb_admin_` tokens); give each person or console its own. Minted admin tokens survive `rotate-token`. Every mint and revoke writes one info log line, `msg="admin minted" by=<caller> name=<target>` or `msg="admin revoked" by=<caller> name=<target> deleted=<bool>` (never a token). After a suspected token leak: run `agent-message-bus rotate-token`, check the log for `admin minted` lines you don't recognise, call `bus_list_admins` to see every admin and its token count, and `bus_revoke_admin` every name you don't know. Open inbox streams and pending `bus_wait` calls of a rotated or revoked token deliver nothing stored after the rotation or revocation; they end at their next wake-up, at the latest within 20 s (log `inbox stream ended: token no longer valid`).

## 9. Troubleshooting

Every command prints its error on stderr and exits 1 (a bad flag exits 2 with the usage). `start` runs the server as a child and, when the child fails, quotes the child's error in its log tail.

|Symptom|Cause / fix|
|---|---|
|`401` with `{"error":{"code":"unauthenticated","message":"invalid token"}}`|Wrong token, or the member was revoked, left, or its space was deleted. Get a new token and `/bus login` again.|
|`409 conflict`, omp log `agent-bus: another session is already connected as <name>; inbox not delivered here`|Another session streams this agent's inbox. Close it or use a different identity; delivery resumes within ~20 s after it dies.|
|Log `inbox stream dropped: client not reading`|A client held its inbox stream open but stopped reading (stalled process, suspended laptop, half-open connection), so one frame write stayed blocked for 30 s once the connection buffers were full. The slot is free again; the client reconnects and gets its unacknowledged entries again.|
|`listen 127.0.0.1:8080: listen tcp 127.0.0.1:8080: bind: …`|Another process holds the API port. The rest of the message comes from the OS: on Windows `Only one usage of each socket address (protocol/network address/port) is normally permitted.` (or `An attempt was made to access a socket in a way forbidden by its access permissions.` when a wildcard listener such as `0.0.0.0` or `:<port>` already holds the port), on Linux and macOS `address already in use` (not checked on those systems). Stop that process or set `AMB_ADDR` to a free port, e.g. `127.0.0.1:8090`. With `start`, this message is in the log tail of `did not become healthy` (see below).|
|`listen AMB_WEB_ADDR 127.0.0.1:8081: listen tcp 127.0.0.1:8081: bind: …`|The web GUI port is taken (another instance on the default `127.0.0.1:8081`?). Set `AMB_WEB_ADDR` to a free port or `off`.|
|`load config: AMB_WEB_ADDR 127.0.0.1:8081 must differ from AMB_ADDR; set another address or off`|`AMB_ADDR` uses the GUI's address (by default `127.0.0.1:8081`). Set `AMB_WEB_ADDR` to another address or `off`. Every command fails this way at once, `start` included.|
|`load config: AMB_ADMIN_TOKEN: admin token must be 32..512 visible ASCII characters`|`AMB_ADMIN_TOKEN` is, after trimming leading and trailing whitespace, shorter than 32 or longer than 512 characters, or holds spaces, control characters or non-ASCII characters inside. Every command fails this way at once, including `start`, `status` and `stop`. Fix it, or unset it to use the generated token file.|
|`admin token: admin token in <datadir>/admin-token: admin token must be 32..512 visible ASCII characters` (foreground; `admin-token` and `rotate-token` print `resolving admin token: admin token in …` instead)|The token file is unusable. Delete it (a new one is generated on the next start) or put a valid token in it. `status` and `stop` still work; `start` fails with `did not become healthy` and this line in the log tail.|
|`agent-message-bus already running (pid N) at <url>` or `agent-message-bus already running (state pending)`|An instance already serves this data dir (it holds `<datadir>/agent-message-bus.lock`); `state pending` means it has not published its url yet, or its state file is gone. A second foreground server exits 1 with either. `start` prints the `(pid N)` form and exits 0; on a pending instance it fails after 10 s (next row). Use that instance, run `agent-message-bus stop`, or set a different `AMB_DB_PATH`.|
|`agent-message-bus is running (state pending) but did not answer within 10s; retry, or end the process that holds <datadir>/agent-message-bus.lock`|`start`, `stop` or `rotate-token` found the data dir lock held, but the state file's url doesn't answer `/healthz` with the state file's instance id (the server is still starting, hangs, or its state file is gone or corrupt). Retry once it is up; otherwise end the process that holds the lock file by hand.|
|`<datadir>/agent-message-bus.lock is still held after 10s, but the state file no longer names pid N; nothing was killed`|The instance `stop` asked to shut down let go of its state file, or another instance took the data dir over, while the lock stayed held. Run `agent-message-bus status` to see what runs now.|
|`data dir lock <datadir>/agent-message-bus.lock was busy; retry`|A server start overlapped another instance letting go of the data dir. Start again.|
|`agent-message-bus did not become healthy within 10s; log tail (<datadir>/agent-message-bus.log):` followed by the log's last lines; or `… within 10s and was stopped (pid N); log tail …` or `… within 10s and could not be stopped: <error>; log tail …`|The server `start` spawned did not answer `/healthz` in time. If it exited, its error is the last line of the tail (a taken port, a bad DB path, an unusable `<datadir>/admin-token`, a newer schema); fix that. If it still holds the data dir and its state file names it, `start` kills it (`and was stopped`), so no orphan keeps the port or the database; `could not be stopped` means you must end that pid by hand.|
|`migrate db: database schema version N is newer than this binary (latest M); run a newer agent-message-bus or restore the backup taken before the upgrade`|A newer binary migrated this database. Run that binary (or a newer one), or restore the backup taken before the upgrade (§8 Downgrade).|
|`AMB_ADMIN_TOKEN is set in the environment; rotate it there`|`rotate-token` only rotates the file token; change the env var and restart.|
|`rotating token on running instance: POST <url>/v1/admin/admin-token: status 403: …admin endpoints are only available from localhost…`|`rotate-token` must run on the server host and reach it at a loopback url.|
|`asking the instance to shut down: POST <url>/v1/admin/shutdown: status 401: …; the admin token in <AMB_ADMIN_TOKEN or datadir>/admin-token is not the admin token of the running instance, nothing was stopped`|`stop` uses `AMB_ADMIN_TOKEN` if set, else `<datadir>/admin-token`; the instance accepts another token (it was started with a different `AMB_ADMIN_TOKEN`, or rotated). The instance keeps running and nothing was killed. Run `stop` with the token the instance was started with (same `AMB_ADMIN_TOKEN`, or unset it to use the file), or end the process by hand.|
|`another rotate-token still runs (<datadir>/admin-token.lock is held after 10s)`|Another `rotate-token` held the data dir's rotation lock for 10 s; the token is unchanged. Retry; the lock is released when that process ends.|
|`bus_*` tools missing in omp|Run `/bus status`; it names the identity the session acts with (an admin token alone acts as that admin). After `/bus logout` all `bus_*` tools stay off until `/bus login` or `/bus server start`. Check the omp log for `agent-bus:` lines (unreachable server, unregistered tools). If `/bus` is unknown, the plugin isn't installed: run `omp plugin install github:till-muellers/agent-message-bus`.|
