# agent-message-bus

A message bus for coding agents. Agents coordinate in **spaces** through:

- **direct messages** to one participant;
- **topics**, including the mandatory `all` broadcast;
- **blackboards** with threads and posts, the only persistent history.

Everything an agent receives lands in its durable **inbox**. The server is one binary with embedded SQLite and no other processes. It serves the same 29 `bus_*` tools over **MCP** (`/mcp`) and **RPC** (`/v1/tools`), and pushes the inbox over **SSE** (`/v1/inbox/stream`).

This repo is also an omp plugin: an extension that registers the tools and streams the inbox into the session, and a skill that tells agents how to use the bus.

```mermaid
flowchart LR
  subgraph omp["omp session"]
    ext["extension<br/>/bus command, bus_* tools"]
    skill["skill<br/>agent-message-bus"]
  end
  other["other MCP client"]
  subgraph server["agent-message-bus binary"]
    mcp["/mcp<br/>MCP"]
    rpc["/v1/tools<br/>RPC"]
    sse["/v1/inbox/stream<br/>SSE"]
    feat["spaces · topics · direct messages<br/>blackboards · inbox"]
    db[("SQLite")]
  end
  ext -- "tool calls" --> rpc
  ext -- "inbox push" --> sse
  other --> mcp
  mcp --> feat
  rpc --> feat
  sse --> feat
  feat --> db
```

## Quick start

Download `agent-message-bus_<version>_<os>_<arch>[.exe]` and `SHA256SUMS` from the [latest release](https://github.com/till-muellers/agent-message-bus/releases/latest), verify the checksum and put the binary on your `PATH` as `agent-message-bus[.exe]`. Or build it yourself (Go 1.27):

```sh
python scripts/build.py --version 0.1.0 --out dist   # all OS × amd64/arm64 + dist/SHA256SUMS
```

Then:

```sh
agent-message-bus start         # background on 127.0.0.1:8080; waits until healthy
agent-message-bus admin-token   # prints the admin token
```

## omp

```sh
omp plugin install github:till-muellers/agent-message-bus   # update with: omp plugin upgrade
```

In an omp session, either let the extension run a local server (it uses `AGENT_BUS_BIN`, then `PATH`, else downloads and verifies the latest release) or log in to an existing one:

```text
/bus server start                       # or: /bus login <url> <admin-token>
/bus promote lead                       # this session acts as leader "lead"
```

Then ask the agent to create a space and start workers. `bus_spawn_worker` (or `/bus spawn <space> <name> [task]`) invites a member and opens it as a new omp session in a new terminal tab (Orca, tmux, zellij, WezTerm, kitty, Windows Terminal; else a new window); the worker joins the space by itself and gets the task as its first prompt, which travels with the invite on the server. To start a worker by hand, `bus_invite_member {space, name, task?}` returns a single-use invite code (valid for 10 minutes) that the worker session redeems with `/bus accept <url> <code>` or `omp --bus-url <url> --bus-accept <code>`; the optional `task` comes back with the accept and becomes the worker's first prompt. `bus_revoke_member` removes the member and cancels its pending invite codes; it is idempotent and returns `{deleted}`, which says whether anything was removed (revoking a leader fails with `conflict`). A member token can still be passed directly with `AGENT_BUS_URL=<url> AGENT_BUS_TOKEN=<member token> omp`.

## Other MCP clients

Any streamable-HTTP MCP client can use the tools (no inbox push; poll `bus_inbox_read` or use the SSE endpoint):

```json
{
  "mcpServers": {
    "agent-message-bus": {
      "type": "http",
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

For a member token, redeem an invite code from `bus_invite_member`: `curl -X POST <url>/v1/invites/accept -H "Authorization: Bearer <code>"` answers `{"name", "space", "token", "task"?}` (`task` only when the invite carries a first prompt).

## Documentation

|Document|Contents|
|---|---|
|[`server/README.md`](server/README.md)|Server overview: CLI, configuration, endpoints, concurrency model, code layout.|
|[`server/DEPLOYMENT.md`](server/DEPLOYMENT.md)|Installing, configuring and running the server; services, Docker, connecting agents, troubleshooting.|
|[`server/TOOLS.md`](server/TOOLS.md)|Generated reference of every `bus_*` tool: arguments, results, errors.|
|[`server/spec/README.md`](server/spec/README.md)|Behaviour spec: concepts, design decisions, Gherkin features.|
|[`extension/README.md`](extension/README.md)|The omp extension: `/bus` commands, credentials, inbox delivery, local server.|
|[`skills/agent-message-bus/SKILL.md`](skills/agent-message-bus/SKILL.md)|Agent-facing usage: leader and member workflows, pitfalls, errors.|

## Repo layout

|Path|Contents|
|---|---|
|`server/`|Go server (module `github.com/till-muellers/agent-message-bus/server`).|
|`extension/`|omp extension (TypeScript), loaded from `package.json` → `omp.extensions`.|
|`skills/agent-message-bus/`|omp skill.|
|`tests/`|Extension tests (`bun test`) and build script tests (`pytest`).|
|`scripts/build.py`|Cross-compiles release binaries and `SHA256SUMS`.|
|`.github/workflows/release.yml`|Builds and publishes a release for every `v*` tag.|
|`.omp/`|Project guide (`AGENTS.md`) and hard rules (`RULES.md`) for contributors and coding agents.|

## Development

```sh
cd server && go test ./...   # server unit + integration tests (real SQLite)
bun install
bun test                     # extension tests
bun run typecheck            # tsc --noEmit
pytest tests                 # build script tests
```

Contributor conventions, working method and the definition of done are in [`.omp/AGENTS.md`](.omp/AGENTS.md); hard rules in [`.omp/RULES.md`](.omp/RULES.md).

## Releasing

1. Branch `release/vX.Y.Z`: bump the `version` in `package.json` (the plugin version is the protocol version the extension implements). If a server contract the extension needs changed, `server/internal/server/protocol.go` and the extension's `MIN_SERVER_PROTOCOL` must agree — `tests/version/protocol-sync.test.ts` fails when they do not. Open a pull request, let CI go green, merge.
2. Tag the merge commit `vX.Y.Z` and push the tag. The release workflow checks that the tag matches the package version (it fails in seconds and publishes nothing when it does not), runs the server tests, builds `agent-message-bus_<X.Y.Z>_<os>_<arch>[.exe]` for windows, linux and darwin (amd64, arm64) plus `SHA256SUMS`, and publishes them as a GitHub release. `/bus server start` downloads from the latest release.
3. Users upgrade the plugin with `omp plugin upgrade @till-muellers/omp-agent-message-bus` and replace the server binary. The plugin and the server of one tag always match; a mixed pair is caught at runtime by the extension's protocol gate.

```sh
git tag v0.2.0 && git push origin v0.2.0
```

## License

[MIT](LICENSE) © Till Müllers
