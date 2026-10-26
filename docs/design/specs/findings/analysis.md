# Postponed review findings — analysis and options

Source: `tmp/TODO.md` → "Postponed review findings" (8 items). Scope: analysis and specs only.
Nothing in `server/`, `extension/`, `skills/` or `scripts/` was changed. Output lives in `docs/design/specs/findings/`.

**Round 2 (PO rulings applied).** Item 3 is **approved** (see "Decision (PO, round 2)" in its section). Item 8 stays on **GitHub distribution, no npm** (see "Decision (PO, round 2)" and R1–R5 in its section). A note on the round-1 side effect outside the repo is at the end of this document.

**Rating legend.** Each option is rated on three axes, 1–5:
*Clean* — how well it fits the existing seams and conventions (5 = drops into the current design).
*Simple* — implementation effort and risk (5 = a small, local, low-risk change).
*Powerful* — how much capability/robustness it buys for the future (5 = opens a seam others can use).

**Feature files** (behaviour items, written in the style of `server/spec/*.feature`):

|File|Covers|Status|
|---|---|---|
|`invite-task-field.feature`|Item 3 — the invalid `task` error names the `task` field; other text fields keep their messages.|**Final** (PO-approved); header lists the changed lines.|
|`invite-cancel.feature`|Item 6 — **moved** (PO-3): only a pointer remains; cancelling is a GUI-only web endpoint by invite id, `docs/design/specs/web/gui-invite-cancel.feature`.|Decided (round 6).|
|`plugin-version-skew.feature`|Item 8 — version/protocol handshake, spawn-time skew check, GitHub release rules.|Rewritten for the GitHub-only flow.|

---

## Item 1 — `SessionCredentials` is shared mutable state

### Current state

`SessionCredentials` is a plain interface with three optional fields (`extension/lib/credentials.ts:36-40`):

```ts
export interface SessionCredentials {
	override?: ActiveCredentials;
	env?: ActiveCredentials;
	accepted?: ActiveCredentials;
}
```

It is mutated in place from four places, and read in two:

|Field|Write sites|Read sites|
|---|---|---|
|`override`|`extension/index.ts:108` (shutdown), `extension/lib/accept.ts:27` (accept), `extension/lib/command.ts:69` (login), `:82` (promote — the only *set*), `:91` (logout)|`extension/lib/credentials.ts:78` (`resolveActive`)|
|`env`|`extension/index.ts:89` (construction — the only *set*), `extension/lib/accept.ts:28`, `extension/lib/command.ts:92`|`extension/lib/credentials.ts:79`|
|`accepted`|`extension/lib/session-identity.ts:43,46,47,50,54,58` — **one writer**, as the TODO says|`extension/lib/credentials.ts:80`|

The asymmetry is real: `accepted` has a single owner module (`session-identity.ts`, which also owns the per-session file `lib/session-store.ts`), while `override` has five write sites across three modules and `env` three across three modules. `resolveActive` (`credentials.ts:77-83`) reads all three plus the file, so the precedence rule is spread over "whoever wrote last".

`extension/lib/accept.ts:27-28` is the sharpest case: accepting an invite *clears* `override` and `env` from a struct it only received as `deps.session` — a module mutating another module's state, and the reason `accept.ts` needs the whole credential set rather than one operation on it.

### Options

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. One owner module `lib/session-credentials.ts`** exposing the struct plus named mutators (`setEnv`, `clearEnv`, `promote`, `clearOverride`, `setAccepted`, `clearAccepted`); `session-identity.ts` keeps owning the *persistence* of `accepted` and calls into the owner.|5|4|4|Mirrors the pattern that already works for `accepted` and gives every field exactly one writer.|
|**B. Funnel writes through functions in `credentials.ts`** (`setOverride(creds, v)`, `clearOverride(creds)`) while the struct stays mutable and public.|3|5|2|Smallest diff, but the mutable struct stays shared, so the "who may write" rule is convention, not structure.|
|**C. Immutable state + derivation** — one state object owned by the session, `resolveActive` derives from a snapshot; writes produce new state passed along.|4|2|5|Removes shared mutable state entirely, but touches `connection.ts`, `command.ts`, `accept.ts`, `index.ts` and every test that pokes `credentials.override`.|

### Recommendation

**A.** The TODO asks for "one owner the same way" as `accepted`; A is that, one level up: one module owns the credential set, the others request mutations by name. It also gives `accept.ts` a narrow dependency (`accept` instead of the whole struct).

**Explicit trade:** the stated requirement is "one owner for `override`/`env`". Option A delivers that by making *one module own all three fields*, which is slightly more than asked — it trades "each field keeps its own owner" for "the credential set has one owner", because `resolveActive` already treats the three fields as one ordered set and `accept.ts` already mutates two of them together. If the PO prefers per-field ownership, the cheap variant is: `session-identity.ts` keeps `accepted`, a new `lib/session-credentials.ts` owns `override` and `env`, and `resolveActive` takes both objects.

## Item 2 — spawn files in the flat `extension/lib/`

### Current state

`extension/lib/` holds 27 top-level `.ts` files plus `launch/` (15) and `shared/` (2) — 44 in total. The three spawn files sit at the top level next to unrelated concerns:

* `extension/lib/spawn.ts` — invite + launch orchestration (65 lines).
* `extension/lib/spawn-tool.ts` — the `bus_spawn_worker` tool definition (49 lines).
* `extension/lib/plugin-args.ts` — forwards the leader's omp extension flags to the worker (37 lines).

They are referenced from `extension/index.ts:42,46,47`, from `extension/lib/command.ts:12` (types only) and from the doc comments at `extension/index.ts:6` and `extension/lib/command.ts:29`. The existing `extension/lib/launch/` folder is *not* a general "worker start" folder: it holds the terminal launchers, and `extension/lib/launch/launcher.ts:22` defines their contract ("All environment probing (env, platform, PATH) happens in `detect`").

`plugin-args.ts` is the odd one out: it is argv plumbing for the worker command line, used only by `spawn.ts` (`spawn.ts:17` references it in the doc).

### Options

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. New folder `extension/lib/spawn/`** holding `spawn.ts`, `spawn-tool.ts`, `plugin-args.ts` (and `tests/spawn/` mirroring it).|5|5|3|One folder = one responsibility ("start a worker"), consistent with `launch/` and `shared/` already existing.|
|**B. Move them into the existing `extension/lib/launch/`.**|2|4|3|Moves files without inventing a seam, but mixes "how to open a terminal" (`launch/*`) with "invite a member and start it" (`spawn.ts`) — and `launch/launcher.ts:22` then no longer describes the whole folder.|
|**C. Leave the files flat and only rename them with a `spawn-` prefix.**|2|5|1|Cheapest, but the folder stays at 27 unrelated files; the TODO's complaint (discoverability) is only cosmetically addressed.|

### Recommendation

**A.** The move is mechanical (3 imports in `extension/index.ts`, 1 type import in `command.ts`, 2 test imports, plus doc comments at `index.ts:6` and `command.ts:29`).

**Explicit trade:** the TODO offers "`extension/lib/launch/` **or** a new `lib/spawn/`". Option A trades the smaller diff of B for a folder whose name matches its contents; `launch/` would otherwise mix the *terminal* concern with the *bus protocol* concern (invite, task, member name), and the launcher contract comment would become misleading. If the PO prefers B, keep `plugin-args.ts` in `launch/` and move only `spawn.ts` + `spawn-tool.ts` — but then `spawn.ts` still has argv plumbing split across folders.

## Item 3 — the invalid invite `task` error names the wrong field

### Current state

`server/internal/invites/service.go:56` validates the task with the *body* validator:

```go
if err := domain.ValidBody(*task); err != nil {
```

`domain.ValidBody` (`server/internal/domain/text.go:31-36`) hardcodes the field name in its message:

```go
return apperr.New(apperr.Invalid, "body must be 1..%d bytes", MaxBodyBytes)
```

So `bus_invite_member` with an empty or 65537-byte `task` answers `invalid "body must be 1..65536 bytes"`. The behaviour is currently *specified that way* — `server/spec/invites.feature:34-42` asserts exactly that message, so this is a spec change, not only a code change.

Callers of the shared helpers today:

|Helper|Callers|
|---|---|
|`domain.ValidBody`|`invites/service.go:56` (task — wrong name), `messaging/service.go:54` (body — correct), `blackboards/posts.go:16` (body — correct), `blackboards/threads.go:60` (body — correct)|
|`domain.ValidTitle`|`blackboards/threads.go:57` (title — correct)|
|`domain.ValidName`|`spaces`/`auth`/`invites` — already field-aware (`domain/text.go:23-28`, "space must match …")|

`domain` is shared code: `internal/domain` holds "business rules shared by several features" and `.omp/AGENTS.md` requires asking before touching `internal/platform/`, `internal/domain/`, `extension/lib/shared/`. The TODO therefore asks for the decision, not the change.

### Options

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. Add `domain.ValidText(field, v string) error`** → `"<field> must be 1..65536 bytes"`; `ValidBody`/`ValidTitle` become thin wrappers (`ValidBody(v) = ValidText("body", v)`) so all existing messages are unchanged. Invites call `domain.ValidText("task", *task)`.|5|4|4|Matches the existing `ValidName(field, v)` convention; one helper for every text field.|
|**B. Fix it inside the `invites` package**: a local `validTask(v)` returning `apperr.New(apperr.Invalid, "task must be 1..%d bytes", domain.MaxBodyBytes)`.|3|5|2|No shared-code change, so no PO gate — but it duplicates the limit rule in a second place and leaves `domain` half field-aware.|
|**C. Generalise the domain helper** to `domain.ValidText(field, v string, max int, unit string)` so byte limits (body, task) and character limits (title) share one function.|4|3|5|One rule for all text; more parameters and a slightly awkward `unit` argument at every call site.|

### Decision (PO, round 2)

**APPROVED: option A.** `server/internal/domain` gains `ValidText(field, v string) error`, which reports `"<field> must be 1..65536 bytes"`. `ValidBody` and `ValidTitle` stay in the package as thin wrappers (`ValidBody(v)` = `ValidText("body", v)`, `ValidTitle(v)` = `ValidText("title", v)` with the character limit), so **no other feature's message changes**. `invites.Service.Create` calls `domain.ValidText("task", *task)`.

**Explicit trade (accepted):** the shared package gains one exported helper so that the error message tells the caller which argument was wrong. The alternative B (a local `validTask` inside `internal/invites`) avoided the shared-code change but duplicated the limit rule and left `domain` half field-aware. Option C (one `ValidText(field, v, max, unit)` for byte *and* character limits) is dropped: no third unit exists, and the `unit` argument would cost every call site.

### Lines that change

|File:line|Change|
|---|---|
|`server/internal/domain/text.go:30-36`|Add `ValidText(field, v)`; `ValidBody` delegates to it. `ValidTitle` keeps the character limit and its own wording (it counts runes, not bytes), delegating to a title-specific path.|
|`server/internal/domain/text_test.go:36-41`|Existing `ValidBody`/`ValidTitle` cases keep their messages; add the `ValidText` cases (empty, too long, at the limit, and one case per field name).|
|`server/internal/invites/service.go:56`|`domain.ValidBody(*task)` → `domain.ValidText("task", *task)`.|
|`server/internal/invites/service_test.go:45-46`|The "empty task" and "long task" cases expect `task must be 1..65536 bytes` instead of `body must be 1..65536 bytes`.|
|**`server/spec/invites.feature:34-42`**|The scenario outline "The task follows the message body rule" expects `invalid "task must be 1..65536 bytes"`. This block is replaced by the "An invalid invite task names the task field" rule of `docs/design/specs/findings/invite-task-field.feature`.|
|~~`skills/agent-message-bus/SKILL.md:81`~~|Dropped: the skill is retired (PO). The guide lists error codes, not messages, so no catalogue line is needed.|

**Unchanged:** `server/spec/direct-messages.feature:51,54,72`, `server/spec/topics.feature:176,178,182` and `server/spec/blackboards.feature:163,164,229` keep `body must be 1..65536 bytes` (their field really is the body); `server/internal/blackboards/service_test.go:51,52,61`, `server/internal/messaging/service_test.go:79,81` and `server/internal/tools/doc_test.go:34` are unaffected; `server/TOOLS.md:110` keeps its "1..65536 bytes" wording for the `task` argument (the limit is unchanged, only the error message is) — but `server/TOOLS.md` is generated, so it is regenerated with `go run ./cmd/tooldoc` and the golden test checked.

## Item 4 — `createSession` is borderline long

### Current state

`extension/index.ts:86-116` — 31 lines, one `return` of an object literal with 9 members, mixing three concerns:

* construction (`:87-92`): client, warn, credentials, identity, view, connection;
* the identity operations delegated to `identity` (`:99`, `:113`);
* the bus operations (`:100-105`: `accept`, `join`) and lifecycle (`:106-109`: `shutdown`).

`createSession` is the only place that knows the whole `Session` shape (`interface Session extends Connection` at `extension/index.ts:68-83`), and `index.ts` is otherwise pure wiring (`createBusExtension` at `:192-206`).

### Options

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. Move `createSession` (and the `Session` interface) into a new `extension/lib/session.ts`**; `index.ts` keeps wiring only.|5|4|3|Matches the documented layout rule ("`extension/index.ts` wires `lib/*.ts`") and gives the session object one module.|
|**B. Split in place** into `createSessionState()` + `createSessionActions()` inside `index.ts`.|3|5|2|No new file, but `index.ts` grows and the two halves stay coupled through the same closure variables.|
|**C. Leave it as is.**|2|5|1|31 lines of mostly one-line delegations is within the ~30-line guideline and reads linearly.|

### Recommendation

**A.** `createSession` is long because it *constructs* a session, and the session concept is not owned by `index.ts` — it is only used there. Moving it makes `index.ts` match its documented role and gives the `Session` type a home next to the modules it composes (`connection.ts`, `session-identity.ts`, `credentials.ts`).

**Explicit trade:** A trades "one file to read to see the whole session" for "one file per concept", i.e. slightly more indirection for a shorter `index.ts`. B keeps everything in one file but grows `index.ts` past 240 lines; C is defensible but leaves the item open with no structural gain — and item 1 (one credential owner) will touch the same neighbourhood, so doing item 4 first makes that change land in a file that only owns wiring.

## Item 5 — on Linux `command()` probes PATH a second time (assessment only)

### Current state

`extension/lib/launch/linux-terminal.ts` is the only launcher that touches PATH in `command()`:

* `detect` (`:21-23`) calls `firstTerminal(ctx)` → up to four `ctx.which()` calls (`:10-17`).
* `command` (`:25-32`) calls `firstTerminal(ctx)` **again** (`:26`), falling back to `TERMINALS[0]` (`x-terminal-emulator`) when nothing is found.

The stated contract is `extension/lib/launch/launcher.ts:22`: *"All environment probing (env, platform, PATH) happens in `detect`; `command()` never fails because of the environment."* — which `linux-terminal` violates in letter (it probes in `command()`) though not in spirit (it never fails because of the environment; it names `x-terminal-emulator` and lets the launch fail).

The cost is one extra PATH sweep (≤4 stat-per-dir calls) per spawn — negligible. The comment at `linux-terminal.ts:24` shows the intent: *"detect found a terminal; if PATH changed since, the first one is named and its launch fails instead."*

Assessment conclusion: **this is a contract/implementation mismatch, not a performance problem.** It is only worth changing if the launcher contract itself changes, exactly as the TODO says.

### Options (for the day the contract changes)

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. Keep the behaviour, correct the contract text** in `launcher.ts:22` to say a launcher may re-probe in `command()` but must not fail because of it.|4|5|1|One comment change; makes the doc match reality.|
|**B. Change the contract to `detect` → prepared command** ("detect resolves the launcher, `command()` renders it"), removing the second probe entirely.|5|2|5|Cleanest long-term shape, but all 9 launchers and their 12 test files change.|
|**C. Memoise `which` in the `LaunchContext`** built at `extension/index.ts:198` (`findOnPath(name, env.PATH)`), so the second sweep is free and the "probing happens in detect" wording stays true in effect.|4|4|2|Small, local, keeps the current launcher implementations untouched.|

### Recommendation

**No change now** (the item is an assessment, and the double probe is cheap and deliberate). If the contract text is ever touched, take **C** as the cheap fix and keep **B** as the target shape for a future launcher redesign.

**Open technical note for whoever implements C:** the memoisation must key on the terminal name *and* be invalidated if PATH changes — the current comment at `linux-terminal.ts:24` explicitly wants a *changed* PATH to be noticed. A per-spawn cache (fresh per `LaunchContext`) is safe; a process-wide cache is not.

## Item 6 — no way to cancel a single unaccepted invite code

### Current state

An invite code is stored hashed (`invites/service.go:74`, `r.hash = auth.Hash(normalize(code))`; primary key `hash` in `server/migrations/0002_invites.up.sql`). It lives 10 minutes (`invites/service.go:20`, `TTL = 10 * time.Minute`) and is deleted on redemption (`repository.go:33-47`, `takeInvite`).

The only cancellation path is `bus_revoke_member` → `spaces.Service.Revoke` (`spaces/revoke.go:16-28`) → the `RevocationHook` (`spaces/membership.go:40-45`) → `invites.Revocation.OnRevoke` (`invites/revocation.go:13-16`) → `deleteForName` (`invites/repository.go:58-73`), which deletes **every** pending code of that name in that space. There is no operation that removes one code, and no way to list pending codes.

Consequence: a leader who issued a code and then regrets it must revoke the member name — which also deletes the member's tokens, inbox entries and blackboards (`spaces/revoke.go:30-55`) — or wait 10 minutes. A leader who wants to cancel *one* of several codes for the same name cannot, and `/bus spawn` never shows the code to the leader (`extension/lib/spawn.ts:4-6`, "the code is never returned"), so the leader cannot even cancel by value in the spawn path.

### Options

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**A. New tool `bus_cancel_invite {space, code}`** — leaders of that space and admins; the server hashes the code, deletes that one row, answers `{"cancelled": true\|false}`. No schema change.|4|5|3|Serves the stated need with one endpoint; the leader already holds the code from `bus_invite_member` (`tools/invites.go:26-28`).|
|**B. `bus_cancel_invite {space, name}`** — cancels all pending codes of a name without touching the member.|3|4|2|Easier to call (no secret), but it cannot address *one* code when a name has several, so it does not meet the requirement.|
|**C. Introduce a short opaque invite `id`** returned next to the code by `bus_invite_member`; cancel by `{space, id}`; the same id enables `bus_list_invites` (which pending codes exist) and the web GUI's pending-invite list (`docs/design/specs/web/invites.feature`).|5|2|5|The right long-term seam: a cancel handle that is not the secret, plus listing. Needs a new column and index in a numbered migration after `0001_schema` (never folded into it; `docs/design/specs/REVIEW.md` §11).|

### Recommendation

**Decided (PO-3, round 6): neither A nor B as a tool.** Cancelling an invite is not a bus tool: no `bus_cancel_invite` for omp or MCP agents, and the registry stays at 34 server tools. Cancelling exists only as a web API endpoint of the GUI, which lists pending invites and cancels one by an opaque invite **id** (option C's id, added by a numbered migration after `0001_schema`). `/bus spawn` exposes no cancel handle. Specified in `docs/design/specs/web/gui-invite-cancel.feature`; `invite-cancel.feature` here is a pointer.

(Round-2 trade-off kept for the record: A re-transmitted the secret code as a tool argument; the id avoids that.)

## Item 7 — only the Orca launcher has been tested for real

### Current state

`extension/lib/launch/` implements 9 launchers; `extension/lib/launch/index.ts:16-26` fixes the detection order (tmux, zellij, orca, wezterm, kitty, windows-terminal, macos-terminal, linux-terminal, windows-console). The test suite has 12 files in `tests/launch/`, and they are **argv-unit tests only** — they assert the `{command, args, detached}` triple, never that a terminal really opens.

What is available on this machine (Windows 10/11, `win32 x64`):

|Launcher|Available here?|Notes|
|---|---|---|
|`orca`|Yes|Installed at `C:\Users\tillm\AppData\Local\Programs\orca`; `orca` CLI on PATH; the launcher `detect` needs `TERM_PROGRAM=Orca` (`orca.ts:17`).|
|`zellij`|Yes|0.45.1 on PATH; `detect` needs `ZELLIJ` (`zellij.ts:9`).|
|`tmux`|Partly|The `tmux` on PATH is **psmux 3.4.0** (`~/.cargo/bin/mux.exe`), a Windows clone, not GNU tmux. WSL2 Ubuntu has real tmux 3.4 (`/usr/bin/tmux`) but no GUI terminals.|
|`kitty`|No|Not installed (winget/choco candidate).|
|`wezterm`|No|Not installed.|
|`windows-terminal`|Yes|`wt.exe` present at `%LOCALAPPDATA%\Microsoft\WindowsApps\wt.exe`.|
|`macos-terminal`|No|No macOS host.|
|`linux-terminal`|Partly|WSL2 exposes `DISPLAY=:0` and `WAYLAND=wayland-0` (WSLg) but no `x-terminal-emulator`/`gnome-terminal`/`konsole`/`xterm` installed.|

### Verification plan

**Tier 0 — CI, no GUI, no session (fully automatable now).**
1. Table-driven *contract* test over all 9 launchers: `detect()` is false without its environment marker; `command()` returns a `detached` flag consistent with the launcher; every launcher produces a non-empty `command`; no launcher returns the worker's argv as a shell string except `orca`/`macos-terminal` (which need quoting).
2. *No-fallback* test: `pickLauncher` with all launchers failing returns `undefined` → `spawnWorker` throws `no terminal launcher found for this environment` **before** any invite is created (`extension/lib/spawn.ts:57-63`). This is the property that keeps a failed spawn from burning an invite code, and it is currently untested.
3. *Argv golden* test for `spawnWorker` with a fake launcher: assert the exact argv, including `--bus-url`/`--bus-accept` ordering and that the code never appears in the returned result (`spawn.ts:63-65`).

**Tier 1 — CI, real terminal, headless.**
4. `tmux`: real end-to-end in a Linux container/CI job (`tmux new-window -n <name> -- <cmd>`); assert the child ran and its argv matched. Available in CI (`ubuntu-latest` has tmux) and here via WSL2.
5. `kitty`: needs a running kitty with `allow_remote_control` → `xvfb-run` + `kitty --listen-on` on a Linux CI runner; `kitty @ launch` then asserts the tab opened.
6. `wezterm`: needs a running wezterm GUI server → same trick (`wezterm start` under `xvfb`) or the `wezterm cli spawn` server mode; Linux CI.
7. `zellij`: `zellij action new-tab` needs a live session; in CI start one (`zellij --session ci`) in a pty, then act on it. Fragile but doable; otherwise manual.

**Tier 2 — CI, platform terminals.**
8. `windows-terminal`: `wt.exe new-tab -- cmd` on a `windows-latest` runner (the image ships Windows Terminal; confirm) — a real UI runner, so flaky; keep the argv test as the gate.
9. `macos-terminal`: `osascript` on `macos-latest`; AppleScript UI automation is unreliable in CI → treat as manual smoke on a developer Mac.
10. `linux-terminal`: `x-terminal-emulator`/`gnome-terminal`/`konsole` under `xvfb` on Linux CI (install the candidates), asserting the first-found terminal is used (`linux-terminal.ts:3-8`).

**Tier 3 — manual smoke, per developer machine (this one included).**
11. `orca` (already the reference), `zellij`, `windows-terminal`, `windows-console` (powershell `Start-Process` quoting, `windows-console.ts:11-22` — the PowerShell 5.1 quoting is the most fragile of all 9).

**The technique that makes Tier 1/2 cheap:** do not launch the real `omp` (it needs a model and a token). Launch a harmless sentinel — e.g. `omp --version`, or better a tiny script that writes its argv to a file — and assert the sentinel ran with the expected argv. That proves "a new terminal session really started and really received our argv", which is exactly what the argv unit tests cannot prove.

**Recommended gate:** Tier 0 in every CI run (fast, deterministic); Tier 1 on `ubuntu-latest` (tmux + kitty + zellij); Tier 2/3 as a documented manual checklist, because the terminal emulators that matter are exactly the ones CI cannot drive reliably.

## Item 8 — publish the updated omp plugin; installed 0.1.0 workers exit on unknown flags

### Current state (evidence)

* **The plugin is distributed as a git dependency (the ruled-out-forever shape).** `package.json` (repo root) declares `"name": "@till-muellers/omp-agent-message-bus"`, `"version": "0.1.0"`, `"omp": {"extensions": ["./extension/index.ts"]}`, `"files": ["extension","skills","README.md","LICENSE"]`. There is no registry package (`registry.npmjs.org` returns **404** for that name), and the local install is a git dependency: `~/.omp/plugins/package.json` has `"@till-muellers/omp-agent-message-bus": "github:till-muellers/agent-message-bus"`, pinned by `~/.omp/plugins/bun.lock` to a commit sha with tag `till-muellers-agent-message-bus-<sha>`. Publishing = merging to `main`; `omp plugin upgrade` re-resolves the git dependency.
* **The version does not move, so nothing can tell stale from fresh.** `package.json` still says `0.1.0` *after* the invite-codes change, so `omp plugin list` reports `0.1.0` for both the old and the new copy — the one field a user would look at is the one field that does not change.
* **The crash is real and reproducible.** The flags `--bus-url`/`--bus-accept` are registered by the extension (`extension/index.ts:200-201`). omp rejects unknown flags **before** any extension loads: `omp --definitely-unknown-flag` → `Error: unknown flag: --definitely-unknown-flag`, **exit code 2**. Verified on this machine with the stale copy installed (commit `0509c0d`, which predates the invite flags): the worker exits immediately, and the leader sees a terminal that opens and closes.
* **`omp plugin upgrade` fixed it here.** After the upgrade the installed copy is commit `d5eaff0` (`~/.omp/plugins/bun.lock`, `.bun-tag`) and `omp --bus-accept … --no-session` no longer errors with "unknown flag" (it now fails later, on "interactive mode requires a terminal"). Note for the PO: that upgrade ran with `--dry-run` and **still performed the reinstall** — omp's dry-run does not cover the npm/git plugin path.
* **No release step versions the plugin.** `.github/workflows/release.yml` runs on `v*` tags, runs the Go tests, calls `python scripts/build.py --version <v>` (server binaries only — `scripts/build.py` never touches `package.json`) and creates a GitHub Release with the binaries. The plugin's own version in `package.json` is never bumped by the release flow, and nothing checks that the tag and the package version agree.

### Release steps (as they are today)

1. Merge the change to `main` → the git dependency in `~/.omp/plugins` follows `main`.
2. `omp plugin upgrade @till-muellers/omp-agent-message-bus` → bun re-resolves `github:till-muellers/agent-message-bus` to the new commit and reinstalls (verified working on this machine). The **unqualified** `omp plugin upgrade` reported "All marketplace plugins are up to date" and did *not* move the git-installed plugin — the targeted form is the one that works, and the README must say so.
3. Push a `v0.2.0` tag → `release.yml` builds and publishes the **server** binaries on GitHub Releases. The plugin is *not* part of that release; it ships as the git dependency.

**What the flow lacks today:** step 1 does not move `package.json`'s version, so steps 2–3 leave `omp plugin list` reporting `0.1.0` for a copy that contains the invite codes. R1–R5 below close that gap without changing the distribution channel.

### Decision (PO, round 2)

**Publishing stays as it is: deployment via GitHub.** The plugin is installed as a git dependency (`omp plugin install github:till-muellers/agent-message-bus`) and updated with `omp plugin upgrade`. **No npm.** The npm-based option is withdrawn.

The gap to close is therefore *not* "get the package onto a registry" but: **make the GitHub flow tell stale from fresh.** Today `package.json` still says `"version": "0.1.0"` after the invite-codes change, so `omp plugin list` reports `0.1.0` for both the old and the new copy — the one thing a user would look at is the one thing that does not move.

### What the GitHub flow needs

**R1. Bump `package.json` on every release.** The version is the only field `omp plugin list` shows for a git-installed plugin (it reads the manifest of the installed commit). Bump it in the same commit as the change, so the installed version *is* the code version. `0.1.0` → `0.2.0` for the invite-codes release. This is a one-line change with no infrastructure.

**R2. Derive the extension's protocol version from the package version.** The extension needs to know "am I protocol N?" to detect skew (see S1). Rather than a second constant that can drift, the extension reads its own `package.json` version at load and exposes it. One source of truth; the protocol version moves exactly when the package version moves.

**R3. Tag the release and let the tag carry the version.** `v0.2.0` must match `package.json`'s `0.2.0`; `.github/workflows/release.yml` already triggers on `v*` tags and can fail the build on a mismatch (it currently only builds the server binaries — the plugin needs no build step). The tag makes the commit referenceable, so "which code am I running" has a second, human-readable answer next to the version.

**R4. Document the update command precisely.** The README currently says `omp plugin upgrade   # later, to update`. The working command is the *targeted* one — `omp plugin upgrade @till-muellers/omp-agent-message-bus` — because the unqualified `omp plugin upgrade` reported "All marketplace plugins are up to date" for the git-installed plugin (verified on this machine). The README must name the targeted form.

**R5. Keep the install instruction on the git source.** `omp plugin install github:till-muellers/agent-message-bus` stays; no registry, no token, no scope to own. Optionally the install line can pin a tag (`github:till-muellers/agent-message-bus#v0.2.0`) for reproducible installs — but note that `omp plugin upgrade` on a pinned spec may refuse to move it, so pinning is for reproducibility, not for staying current.

**R6. The extension refuses a server that is too old (round 3, cross-folder review R2).** Plugin and server ship separately, so a mixed pair is possible. Some changes break such a pair silently. Example: with the invite task as a direct message (`docs/design/specs/guidance/first-prompt.feature`), a new extension on an old server loses the task. The old server still returns `task` in the accept response and stores no direct message, and the new extension no longer sends a user message. Fix: the server reports a protocol version (an integer, bumped with every change the extension depends on) as the response header `Agent-Bus-Protocol: N` on `GET /healthz`. The header is additive: the body stays `ok` (`server/spec/transports.feature:20-24`), and `/healthz` needs no token, so the check also works before an invite is redeemed. The extension declares the minimum server protocol it needs. Before it redeems an invite code or activates its tools, it reads the header. If the header is missing or below the minimum, it refuses with a message that names both versions and says to upgrade the server. An older extension against a newer server needs no check: the server keeps its contracts additive or delivers the task through the inbox. Specified in `plugin-version-skew.feature`, rule "The extension refuses a server that is too old".

### Options — version-skew safeguard (within the GitHub constraint)

|Option|Clean|Simple|Powerful|Justification|
|---|---|---|---|---|
|**S1. Version handshake + spawn-time check**: the extension derives its protocol version from `package.json` (R2) and, before spawning, compares it with the version of the plugin the worker will load (the forwarded extension path, else the installed plugin from `omp plugin list --json`). On a mismatch the spawn fails *before* inviting, with the upgrade command in the message.|5|4|5|Uses the version R1 already publishes; catches the skew where it matters (the worker's copy) and never burns an invite code.|
|**S2. Pre-flight capability probe** of the target omp (`omp --help` / `omp --bus-accept x --help`).|1|3|2|**Does not work:** `--help` short-circuits flag validation — `omp --bus-accept probe --help` prints the help and exits 0, so an unknown flag is invisible to a `--help` probe. Verified on this machine.|
|**S3. Deliver the invite without CLI flags** (env var `AGENT_BUS_INVITE_CODE`/`AGENT_BUS_URL`, or a temp file) so an old omp ignores it instead of dying.|4|3|4|Old workers start and simply do not auto-join (leader can then instruct manually) — graceful instead of fatal, but the join becomes two-sided manual work for old clients.|
|**S4. Cap the blast radius**: the leader detects "worker exited immediately" (exit code 2 + no join within a few seconds) and reports "the worker's omp plugin is too old; run `omp plugin upgrade @till-muellers/omp-agent-message-bus`".|3|4|3|No protocol change, but it detects the symptom late and only heuristically.|

### Recommendation

**R1–R5 + S1 (+ S4 as a net).** R1 is the load-bearing change: without a version that moves, neither `omp plugin list` nor S1 can tell stale from fresh, and every other step is decoration. R2 makes the version meaningful to the extension; R3 keeps the tag and the package version in step; R4 tells users the command that actually works.

S1 turns the failure from "a terminal opens and dies with `unknown flag: --bus-accept`" into "the spawn is refused with the exact upgrade command, and no invite code is created". S4 catches the residual case where the installed plugin cannot be determined (S1 then proceeds best-effort, per the spec).

**Explicit trade:** S1 trades "the spawn always tries, and the worker may die" for "the spawn refuses when the worker's plugin is provably too old" — a *different* requirement that yields a better seam, because the crash is caused by the transport (CLI flags), not by the bus protocol. If the PO prefers S3 over S1, the extension would pass the invite by environment variable and keep the flags only as a fallback; `plugin-version-skew.feature` would then be rewritten around "the worker starts, and joins only if it understands the handshake".

**Note on S1's "worker will load" determination:** the worker loads the extension the leader forwards (`pluginArgs`, `extension/lib/plugin-args.ts`) if any, and otherwise the installed plugin. The leader knows both — its own forwarded path and the installed version from `omp plugin list --json` — so the check needs no new server round trip.

**Behaviour spec:** `docs/design/specs/findings/plugin-version-skew.feature` (rewritten for the GitHub-only flow: the release rule now covers the package version, the tag and the targeted upgrade command).

---

## Open questions for the PO (collected)

**Answered in round 2:**

* **Item 3 — RESOLVED.** `domain.ValidText(field, v)` is **approved**, with `ValidBody`/`ValidTitle` kept as wrappers. The final spec is `docs/design/specs/findings/invite-task-field.feature`; the changed lines are listed in the Item 3 section above (notably `server/spec/invites.feature:34-42`).
* **Item 8 (distribution) — RESOLVED.** Publishing stays as it is: **GitHub**, a git dependency installed with `omp plugin install github:till-muellers/agent-message-bus`. **No npm.** The Item 8 section now specifies R1–R5 (version bump per release, protocol version derived from the package version, tag/package-version agreement, the targeted `omp plugin upgrade @till-muellers/omp-agent-message-bus` command) and the safeguard within that constraint.

**Still open:**

1. **Item 1** — Is one owner for *all three* credential fields acceptable, or do you want per-field owners (`session-identity.ts` keeps `accepted`, a new module owns `override`/`env`)?
2. ~~**Item 6**~~ — *Decided (PO-3):* GUI-only cancel by opaque invite id; no tool; `/bus spawn` exposes no cancel handle.
4. **Item 7** — Which launchers must be *supported* (not merely argv-tested)? The proposal keeps tmux/kitty/zellij automated on Linux CI and treats macOS Terminal, WezTerm, kitty GUI and Windows Terminal as manual.
5. ~~**Item 8 (safeguard)**~~ — *Decided (PO-11):* S1, a version check at spawn.
6. **Item 5** — No action proposed; confirm that the contract wording in `launch/launcher.ts:22` may be corrected to match `linux-terminal`'s behaviour at some point.

---

## Process note — side effect outside the repo (round 1)

While gathering evidence for item 8 I ran `omp plugin upgrade @till-muellers/omp-agent-message-bus --dry-run` on this machine. **omp's dry-run does not cover the git/npm plugin path: the reinstall happened anyway.** The user's installed plugin changed outside the repository:

|What|Before|After|
|---|---|---|
|Installed commit (`~/.omp/plugins/node_modules/@till-muellers/omp-agent-message-bus/.bun-tag`)|`till-muellers-agent-message-bus-0509c0d`|`till-muellers-agent-message-bus-d5eaff0`|
|Pin in `~/.omp/plugins/bun.lock`|`…#0509c0d`|`…#d5eaff0`|
|Installed extension files|no `lib/accept.ts`, `lib/invite-flag.ts`, `lib/spawn.ts`, `lib/spawn-tool.ts`, `lib/plugin-args.ts`, `lib/session-identity.ts`, `lib/launch/`|those files present (the invite-codes feature)|

The reported `package.json` version stayed `0.1.0` in both revisions — which is exactly the "stale and fresh look identical" gap that R1 closes.

**No repository file was touched** (`git status` clean; `tmp/` is gitignored and holds only this analysis). The change to `~/.omp/plugins` is the user's installed plugin and was not reverted — it is the state the item asks for (the installed copy now contains the invite codes), and reverting it would re-break the reproduction. From round 2 onwards, no commands with side effects outside `tmp/` are run.
