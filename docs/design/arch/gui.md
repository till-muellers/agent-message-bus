# Web GUI architecture

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix (e.g. `web/gui-access.feature`) are relative to `docs/design/specs/`.

Owner: Arch-client. Specs: `docs/design/specs/web/*.feature`, `docs/design/specs/web/analysis.md` §3.3-3.11. PO decisions: **(b) an embedded strict-TS SPA with no framework** (PO-10); one server log line per GUI change, no audit view (PO-1); space delete behind a confirm (PO-4); **pending invites are listed and cancelled by invite id through a web endpoint; there is no `bus_cancel_invite` tool** (PO-3). Server routes and shapes: `docs/design/arch/contracts.md` §10 (Arch-server), summarised in §4.

---

## 1. Principles

- **No runtime dependency.** Browser APIs only (`fetch`, `URL`, `history`/`hashchange`, DOM). Toolchain = the repo's existing one: bun (build, test), TypeScript (`tsc --noEmit`).
- **The browser never holds a token.** The admin/agent token is typed once into a password field, sent in `Authorization: Bearer` to `POST /web/session`, and the field is cleared. The session is an HttpOnly cookie; the CSRF token lives in memory only (web analysis §3.7, §4.3).
- **No HTML from data.** Every text node is created with `textContent`/`createTextNode`; `innerHTML`, `outerHTML`, `insertAdjacentHTML` and `document.write` are banned (fitness test, §7). Message bodies and post bodies are user-controlled.
- **Pure core, thin DOM edge.** Routing, state, API decoding, polling schedule and views are pure TypeScript and unit-tested with `bun test` without a DOM. Only `view/mount.ts`, `router/hash-source.ts` and `main.ts` touch browser globals.
- **One responsibility per file, small functions** (`.omp/RULES.md`).

---

## 2. Layout: `gui/` at the repo root

```
gui/
  package.json          private; scripts build/test/typecheck; devDependencies typescript + @types/bun (same versions as root); no dependencies
  bun.lock              its own lockfile
  tsconfig.json         strict; lib ESNext + DOM; types bun; include src, tests, scripts
  index.html            shell template: <div id="app">, one <script type="module"> placeholder replaced by the build
  scripts/
    build.ts            Bun.build → <out>/assets/main-<hash>.js; writes <out>/index.html; --out (default ../server/internal/web/dist)
  src/
    main.ts             boot: store + api + router + poller + mount
    api/
      http.ts           fetch wrapper: same-origin credentials, JSON, X-CSRF-Token on non-GET, error envelope → ApiError
      decode.ts         narrowing decoders for every response (unknown → typed); no `as` casts on wire data
      types.ts          wire types from contracts.md (Identity, SpaceSummary, Overview, Invite, ThreadList, Thread, Entry, Admin)
      session.ts        signIn(token), restore(), signOut()
      reads.ts          state(), spaceState(space), threads(space, board), thread(space, id), inbox(after?)
      admins.ts         (W2) listAdmins(), mintAdmin(name), revokeAdmin(name)
      mutations.ts      (W3) invite, cancelInvite(space, id) (DELETE), revoke, post, sendTopic, deleteSpace
    state/
      app-state.ts      AppState, Route, Resource<T> types
      actions.ts        Action union
      reducer.ts        pure (state, action) → state
      store.ts          getState / dispatch / subscribe (~30 lines)
    router/
      routes.ts         parseRoute(hash) → Route; hrefOf(route) → "#/…" (pure)
      hash-source.ts    hashchange → dispatch(navigate)
    poll/
      poller.ts         polls the current route's resource every 5 s while visible; backoff on errors; injected clock
    view/
      h.ts              h(tag, props, ...children) → VNode (plain objects)
      mount.ts          VNode → DOM with a keyed patch (reuses nodes; keeps focus and input values)
    views/
      layout.ts         header (signed-in name/kind, nav, sign out), error banner
      login.ts          token form
      spaces.ts         space list with counts (participants, invites)
      space.ts          participants, topics, blackboards, open invites
      board.ts          threads of a blackboard (most recently active first)
      thread.ts         a thread: title, posts with author and time
      inbox.ts          the signed-in agent's own inbox, read-only
      admins.ts         (W2) admin list, mint form, revoke button
      confirm.ts        (W3) confirm dialog naming space / target / role
      shown-once.ts     (W2/W3) invite code or admin token shown once
      forms.ts          (W3) invite, post, send-to-topic forms
    format/
      time.ts           relative and absolute times
  tests/                bun test, one file per src module
```

Why a separate package (web analysis §3.6): the root `package.json` **is the omp plugin** (`@till-muellers/omp-agent-message-bus`, installed from GitHub), and the GUI is not part of the plugin; the Go module must stay free of `node_modules`. `gui/package.json` adds no new package names (only `typescript` and `@types/bun`, already root devDependencies), so it needs no extra approval beyond PO-10. The root `tsconfig.json` keeps `include: ["extension", "tests"]` (unchanged), so the root `tsc` and `bun test tests/` never see `gui/`.

`gui/tsconfig.json` (key options): `strict`, `noEmit`, `allowImportingTsExtensions`, `verbatimModuleSyntax`, `isolatedModules`, `noUncheckedIndexedAccess`, `lib: ["ESNext", "DOM", "DOM.Iterable"]`, `types: ["bun"]`, `moduleResolution: "bundler"`. Imports use `.ts` extensions (`.omp/AGENTS.md` TypeScript rules).

---

## 3. Routing and state without a framework

### 3.1 Routes (hash routing)

Hash routing means the server serves only `/web/` (index.html) and `/web/assets/*`; no SPA fallback route on the Go side (agreed with Arch-server).

| Hash | Route | Resource polled |
|---|---|---|
| `#/` | `{name: "spaces"}` | `GET /web/api/state` |
| `#/spaces/<space>` | `{name: "space", space}` | `GET /web/api/state?space=<space>` |
| `#/spaces/<space>/boards/<board>` | `{name: "board", space, board}` | `GET /web/api/spaces/<space>/blackboards/<board>/threads` |
| `#/spaces/<space>/threads/<id>` | `{name: "thread", space, threadId}` | `GET /web/api/spaces/<space>/threads/<id>` |
| `#/inbox` | `{name: "inbox"}` | `GET /web/api/inbox` |
| `#/admins` | `{name: "admins"}` (W2) | `GET /web/api/admins` |
| anything else | `{name: "not-found"}` | — |

`parseRoute` decodes with `decodeURIComponent` and validates names against the bus name rule (`^[a-z0-9][a-z0-9_-]{0,62}$`); an invalid segment is `not-found` (no request is sent).

### 3.2 State

```ts
// state/app-state.ts
export type Resource<T> = { status: "idle" } | { status: "loading"; last?: T } | { status: "ready"; data: T; at: number } | { status: "error"; error: ApiError; last?: T };
export type SessionState = { status: "unknown" } | { status: "signed-out"; error?: string } | { status: "signed-in"; identity: Identity; csrf: string };
export interface AppState {
	session: SessionState;
	route: Route;
	view: Resource<ViewData>;         // the current route's data; replaced on navigation
	shownOnce?: { label: string; secret: string };  // invite code / admin token; cleared on next navigation (W2/W3)
	confirm?: PendingConfirm;         // W3
}
// state/actions.ts
export type Action =
	| { type: "session/restored"; identity: Identity; csrf: string } | { type: "session/signed-out"; error?: string }
	| { type: "navigate"; route: Route }
	| { type: "view/loading" } | { type: "view/loaded"; data: ViewData; at: number } | { type: "view/failed"; error: ApiError }
	| { type: "secret/shown"; label: string; secret: string } | { type: "confirm/open"; confirm: PendingConfirm } | { type: "confirm/close" };
// state/store.ts
export interface Store { getState(): AppState; dispatch(action: Action): void; subscribe(listener: (s: AppState) => void): () => void; }
export function createStore(reducer: (s: AppState, a: Action) => AppState, initial: AppState): Store;
```

- Render loop: `store.subscribe(state => mount(root, render(state)))`; `render` is `views/layout.ts` choosing the view by `state.route.name`. A `view/loaded` whose data is deep-equal to the current data does not notify (no re-render every 5 s).
- Any `401` from any call dispatches `session/signed-out` (session revoked, rotated token, expired cookie: `gui-access.feature:103-140`). `403` shows the error message in place (e.g. a leader opening `#/admins`).
- `view/mount.ts` patches by tag + `key` prop: same tag and key reuse the node and update props/text; an `<input>`/`<textarea>` that has focus keeps its value. This keeps forms usable while polling re-renders (W3).

### 3.3 Polling

```ts
// poll/poller.ts
export interface Clock { setTimeout(fn: () => void, ms: number): unknown; clearTimeout(id: unknown): void; now(): number; }
export function createPoller(deps: { load(route: Route): Promise<ViewData>; store: Store; clock: Clock; visible(): boolean; intervalMs?: number }): { start(): void; stop(): void; refreshNow(): void };
```

- Interval 5 s (`web/analysis.md` §3.5), paused while `document.hidden`, immediate refresh on navigation and after a successful mutation; on errors back off 5 → 10 → 30 s. No SSE in milestone 1 (`live-updates.feature` header); `GET /web/api/events` would replace the poller later without touching views.
- The GUI never opens `GET /v1/inbox/stream` and never calls `bus_inbox_ack` (`gui-inbox.feature`, `live-updates.feature:63-76`); the inbox view is a read of `GET /web/api/inbox`.

---

## 4. Talking to the server

As in `contracts.md` §10 (checked, every call present). Sign-in errors the login view shows verbatim: 401 `invalid token`/`bearer token required`, 429 `rate_limited "too many failed sign-ins; try again later"` (§10.1).

| Call | Request | Response used by |
|---|---|---|
| Sign in | `POST /web/session`, header `Authorization: Bearer <token>`, no body | `{kind, name, space?, csrf}` + `Set-Cookie amb_session` (`Path=/web`) → `session/restored` |
| Restore after reload | `GET /web/session` (§10.1) | same body incl. the same `csrf`; 401 → login view |
| Sign out | `DELETE /web/session` (CSRF-exempt) | `{ok: true}` |
| Space list | `GET /web/api/state` | `{you, spaces: [SpaceSummary + invites], inbox: {waiting}}` → `views/spaces.ts` |
| Space | `GET /web/api/state?space=<s>` | `{you, overview, invites?, inbox}` → `views/space.ts`; `invites` present only for leader-rights sessions (leader of the space or admin) |
| Pending invites (slice 10) | `GET /web/api/spaces/<s>/invites` | `[{id, name, expiresAt, invitedBy}]` → invite rows with a cancel button; member session 403 `only leaders may list or cancel invites` (section hidden for members) |
| Threads | `GET /web/api/spaces/<s>/blackboards/<b>/threads` | `bus_list_threads` result → `views/board.ts` |
| Thread | `GET /web/api/spaces/<s>/threads/<id>` | `bus_read_thread` result → `views/thread.ts` |
| Inbox | `GET /web/api/inbox?after=&limit=` | `[Entry]` → `views/inbox.ts` |
| Admins (W2) | `GET /web/api/admins`, `POST /web/api/admins {name}`, `POST /web/api/admins/<n>/revoke` | list; `{name, token}` shown once; `{deleted}` |
| Mutations (W3) | `POST …/invites {name, task?}`, `DELETE …/invites/<id>` + `X-CSRF-Token` (→ `{cancelled}`; slice 10, PO-3, `web/gui-invite-cancel.feature:17-19`), `POST …/revoke {name, confirm: "<space>/<name>"}`, `POST …/threads/<id>/posts {body}`, `POST …/topics/<t>/messages {body, critical?}`, `DELETE /web/api/spaces/<s> {confirm: "<s>"}` | per contract; invite code shown once; a `cancelled: false` result shows "the invite was already used, expired or cancelled" and refreshes |

```ts
// api/http.ts
export class ApiError extends Error { constructor(readonly status: number, readonly code: string, message: string) { super(message); } }
export interface Http { get<T>(path: string, decode: Decoder<T>): Promise<T>; send<T>(method: "POST" | "DELETE", path: string, body: unknown, decode: Decoder<T>): Promise<T>; }
export function createHttp(deps: { fetchFn: typeof fetch; csrf(): string | undefined }): Http;
// api/decode.ts
export type Decoder<T> = (json: unknown) => T;   // throws ApiError(0, "invalid_response", …) on shape mismatch
```

- `credentials: "same-origin"`, `Accept: application/json`; non-GET adds `X-CSRF-Token` (except sign-in/out). Errors decode the shared envelope `{"error":{"code","message"}}` (`server/internal/platform/httpx/json.go:11-17`); the page shows the server's message verbatim (the specs assert those texts, e.g. `gui-mutations.feature:101,107`).
- Ordering rules are the server's (participants admins → leaders → members, topics `all` first, threads most recent first: `web/analysis.md` §3.11); the GUI **does not re-sort**.

---

## 5. Views (milestone mapping)

| Milestone | Views | Spec files |
|---|---|---|
| W1 (read-only) | login, layout, spaces, space, board, thread, inbox | `gui-access`, `spaces-and-participants`, `topics`, `blackboards`, `invites`, `live-updates`, `gui-inbox` |
| W2 | admins (list, mint once, revoke) | `gui-admins` |
| W3 | forms (invite, post, send), invite cancel button per pending invite (PO-3), confirm (revoke, delete space), shown-once | `gui-mutations` (PO-1, PO-4) + PO-3 cancel |

Rules visible in the views: the signed-in agent is shown by kind and name ("admin ops"), never a placeholder (`spaces-and-participants.feature:114-118`); a secret (invite code, admin token) is shown once with "it cannot be shown again" and is dropped from state on the next navigation (`gui-mutations.feature:85-88`, `gui-admins.feature:73-76`); the confirm dialog names space, target and role and sends the `confirm` string the server checks.

---

## 6. Build output embedded by Go

- **Owner of the Go side: Arch-server** (`server/internal/web/`: `embed.go`, the handlers, `dist/`). Confirmed by message: `GET /web/` → `index.html`, `GET /web/assets/*` → hashed bundles, no SPA fallback, the server never runs bun.
- `gui/scripts/build.ts`:
  1. `Bun.build({ entrypoints: ["src/main.ts"], outdir: "<out>/assets", target: "browser", minify: true, naming: "[name]-[hash].[ext]", sourcemap: "none" })`;
  2. removes stale `<out>/assets/*` first;
  3. writes `<out>/index.html` from `gui/index.html`, replacing the script placeholder with `/web/assets/main-<hash>.js`;
  4. `--out <dir>` (default `../server/internal/web/dist`), used by Docker (§pipeline).
- **Placeholder (contracts.md §10.5).** `server/internal/web/embed.go` uses `//go:embed all:dist`; tracked are only `server/internal/web/dist/.gitkeep` and `server/internal/web/.gitignore` (`dist/*`, `!dist/.gitkeep`). Without a GUI build the server answers `GET /web/` with a built-in fallback page, so `go build`/`go test` work on a fresh checkout without bun, and a local GUI build leaves `git status` clean.
  - The root `.gitignore` line `dist/` (`.gitignore:2`) matches `server/internal/web/dist/` and would keep `.gitkeep` from being added: W1a changes it to `/dist/` (the release output of `scripts/build.py --out dist`).
- **Dev loop.** `bun run --cwd gui build --watch` + rebuild/restart the Go server (embed is compile-time). A dev-only `AMB_WEB_DIR` (CR-G2) was declined by Arch-server (Q-A7 in `QUESTIONS.md`; `contracts.md` §12); no spec needs it.
- **Headers (contracts.md §10.5).** CSP `default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`, `nosniff`, `no-referrer`; `index.html` no-cache, `assets/*` immutable, `/web/session` and `/web/api/*` no-store. The SPA therefore uses no inline script or style, no `<base>`, and submits forms only through `fetch`.

---

## 7. Test approach (bun test)

`bun test` in `gui/` (its own tsconfig; `describe`/`test` from `bun:test`, sentence-style test names as in `tests/*.test.ts`).

| Module | How it is tested |
|---|---|
| `router/routes.ts` | table-driven: hash → route → hash round trip; invalid names → not-found |
| `state/reducer.ts` | table-driven per action; secret cleared on navigate; 401 → signed-out |
| `api/http.ts`, `api/*.ts` | fake `fetchFn` recording method, path, headers (CSRF present on non-GET, absent on GET), body; error envelope → `ApiError` |
| `api/decode.ts` | valid shapes pass, every missing/wrong field throws `invalid_response` |
| `poll/poller.ts` | fake `Clock`: interval, pause when hidden, backoff, refresh after mutation |
| `views/*.ts` | assert the VNode tree (`h()` output): texts, order, keys, links (`hrefOf`), buttons per kind (no admin nav for a member) |
| `view/mount.ts` | not unit-tested against a real DOM (no new dependency such as happy-dom); covered by the smoke procedure. Its patch decision function (`samePlace(a, b)`) is pure and tested. |
| `scripts/build.ts` | builds into a temp `--out`; asserts `index.html` references an existing `assets/main-*.js` |
| fitness | `tests/no-raw-html.test.ts`: no `innerHTML`/`outerHTML`/`insertAdjacentHTML`/`document.write` in `gui/src` |

Server behaviour (cookie flags, 401/403/405, CSRF binding, loopback rule) is tested in Go by Arch-server's W slices; the GUI tests only the client's side of each contract.

**Smoke procedure (per GUI slice):** throwaway server (`AMB_ADDR=127.0.0.1:<port>`, `AMB_WEB_ADDR=127.0.0.1:<port2>`, `AMB_DB_PATH=<tmp>/x.db`, `AMB_ADMIN_TOKEN=<32+ chars>`), GUI built into `server/internal/web/dist`, `go run .` in `server/`; open `http://127.0.0.1:<port2>/web/` (the GUI listener `AMB_WEB_ADDR`; the API listener `AMB_ADDR` serves no `/web`); sign in with the admin token; seed data with `POST /v1/tools/<name>` on `AMB_ADDR` (`.omp/AGENTS.md` "Running it locally"); check each view, a 5 s refresh after a seeded change, sign-out, and (W3) every mutation's confirm text; DevTools → Application shows no token in storage, cookie `HttpOnly`.
