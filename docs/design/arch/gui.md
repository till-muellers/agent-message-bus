# Web GUI architecture

> **Committed in slice 0.** Design paths use `docs/design/specs/…` and `docs/design/arch/…`. Spec paths without a prefix (e.g. `web/gui-access.feature`) are relative to `docs/design/specs/`.

Owner: Arch-client. Specs: `docs/design/specs/web/*.feature`, `docs/design/specs/web/analysis.md` §3.3-3.11. PO decisions: **(b) an embedded strict-TS SPA with no framework** (PO-10); one server log line per GUI change, no audit view (PO-1); space delete behind a confirm (PO-4); **pending invites are listed and cancelled by invite id through a web endpoint; there is no `bus_cancel_invite` tool** (PO-3). Server routes and shapes: `docs/design/arch/contracts.md` §10 (Arch-server), summarised in §4.

---

## 1. Principles

- **No runtime dependency.** Browser APIs only (`fetch`, `URL`, `history`/`hashchange`, DOM). Toolchain = the repo's existing one: bun (build, test), TypeScript (`tsc --noEmit`).
- **The browser never holds a token.** The admin/agent token is typed once into a password field, sent in `Authorization: Bearer` to `POST /web/session`, and the field is cleared. The session is an HttpOnly cookie; the CSRF token lives in memory only (web analysis §3.7, §4.3).
- **No HTML from data.** Every text node is created with `textContent`/`createTextNode`; `innerHTML`, `outerHTML`, `insertAdjacentHTML` and `document.write` are banned (fitness test, §7). Message bodies and post bodies are user-controlled.
- **Pure core, thin DOM edge.** Routing, state, API decoding, polling schedule and views are pure TypeScript and unit-tested with `bun test` without a DOM. Only `view/mount.ts`, `view/form-field.ts`, `router/hash-source.ts` and `main.ts` touch browser globals (`document`, `window`, `location`, `history`, `navigator`, the storages, `fetch`, `FormData`, the `HTML…Element` classes); `state/*` never imports `views/*`, and the import graph of `gui/src` has no cycle (every relative import resolves to a module). `gui/tests/layout.test.ts` enforces all three.
- **One responsibility per file, small functions** (`.omp/RULES.md`).

---

## 2. Layout: `gui/` at the repo root

```
gui/
  package.json          private; scripts build/test/typecheck; devDependencies typescript + @types/bun (same versions as root); no dependencies
  bun.lock              its own lockfile
  tsconfig.json         strict; lib ESNext + DOM; types bun; include src, tests, scripts
  index.html            shell template: <div id="app">, the placeholders %MAIN_SCRIPT% (<script type="module">) and %STYLE% (<link rel="stylesheet">) replaced by the build
  scripts/
    build.ts            Bun.build → <out>/assets/main-<hash>.js + style-<hash>.css; writes <out>/index.html; --out (default ../server/internal/web/dist)
  src/
    main.ts             boot: store + api + router + poller + mount
    style.css           the one stylesheet (a file, never inline: CSP)
    api/
      api-error.ts      ApiError(status, code, message); errorMessage(unknown); toApiError(unknown)
      http.ts           fetch wrapper: same-origin credentials, JSON, X-CSRF-Token on non-GET, error envelope → ApiError
      decode.ts         narrowing decoder primitives (fields, text, count, flag, list) + session (the kind stays text); no `as` casts on wire data
      decode-state.ts   decoders of GET /web/api/state (space list with mayManage per row and maxInboxLimit, space detail with mayManage; roles stay text); later reads get their own decode-<read>.ts
      decode-threads.ts decoders of the blackboard reads (thread list, thread with posts)
      decode-inbox.ts   decoder of GET /web/api/inbox (entries; the type and the delivery class stay text so newer values still show)
      types.ts          wire types from contracts.md (Identity, SpaceSummary, Overview, PendingInvite, ThreadInfo, Thread, Post, Entry, AdminInfo, MintedAdmin)
      session.ts        signIn(token), restore(), signOut()
      reads.ts          admins() (GUI-5); spaces(), space(name) (GET /web/api/state[?space=]); threads(space, board), thread(space, id), inbox() (GUI-4: reads the state's maxInboxLimit, then that many entries, oldest first; the view says when the list is cut)
      decode-admins.ts  (W2) decoders of the admin list, the minted token and {deleted}
      admins.ts         (W2) mint(name), revoke(name); the list is the polled read reads.admins()
      decode-mutations.ts (W3) decoders of the invite code, {cancelled} and {postId}
      mutations.ts      (W3) invite, cancelInvite(space, id) (DELETE), revoke (confirm "<space>/<name>"), post, sendTopic, deleteSpace (confirm "<space>")
    state/
      app-state.ts      AppState, Route, Resource<T> types
      view-data.ts      ViewData: the current route's data, tagged by view kind
      same-data.ts      deep equality of decoded data (the "no re-render every 5 s" guard)
      actions.ts        Action union
      reducer.ts        pure (state, action) → state
      store.ts          getState / dispatch / subscribe (~30 lines)
      session-intents.ts signIn / restore / signOut: run the session calls, dispatch the outcome (never rejects)
      mutate.ts         mutate(deps, call, done): awaits a mutation, dispatches done(result) and refreshes; resolves true when it went through, false when refused (a form keeps what was typed then); 401 → signed-out, else the server's message as an alert notice (never rejects)
      admin-intents.ts  (W2) mintAdmin → secret/shown, revokeAdmin → notice/shown; every intent resolves like mutate
      space-intents.ts  (W3) invite → secret/shown; cancel, post, send → notice; ask/dismiss open and close the confirmation, confirmed closes it and runs the revoke or delete; a successful delete sends an operator still on a page of that space to the space list (store, then hash) before its notice, so the notice stays and the deleted space is not polled; one who moved on while it ran stays where they are
    router/
      routes.ts         parseRoute(hash) → Route; hrefOf(route) → "#/…" (pure)
      hash-source.ts    hashchange → dispatch(navigate)
    poll/
      load-view.ts      loadView(reads, route): the request behind a route's view (undefined: no data)
      poller.ts         polls the current route's resource every 5 s while visible; backoff on errors; injected clock
    view/
      h.ts              h(tag, props, ...children) → VNode (plain objects)
      plan-patch.ts     planPatch(previous, next) (pure): which old child each new child patches in place (tag + key, keyed moves), which old ones go
      mount.ts          VNode → DOM: applies planPatch per parent (reuses nodes; keeps focus and input values); a newly created node with the `autofocus` attribute takes the focus, and the focus goes back to where it was when such a node is removed while it holds it
      form-field.ts     takeField(event, name): reads and clears a form field (the token never reaches state); submitForm(event, act): hands all fields to act and resets the form only when act resolves true and the form still holds what was sent (a refused submit and text typed meanwhile are kept; a second submit while one is unanswered is ignored)
    views/
      intents.ts        Intents: the one contract of what the operator can do; each view takes the Pick it needs. The state/*-intents.ts modules implement it structurally (state never imports views); main.ts only spreads them into one object
      layout.ts         header (signed-in name/kind, nav, sign out), confirm/outcome section, error banner; the notice is `.outcome`
      login.ts          token form
      table.ts          table(head, rows): a header row and keyed rows, in a `.scroll-x` box so a wide table scrolls inside itself instead of widening the page
      time.ts           time(iso): a <time> element with the local formatted text
      spaces.ts         space list: leader and member counts, admin names, open invites where the session may manage a space (mayManage)
      space.ts          participants, topics + send form (no `all` or critical without leader rights), blackboards, recent threads; with leader rights (the state's mayManage, never the presence of invites) revoke per member (and per leader for admin sessions; no revoke for a role the GUI does not know), open invites with a cancel button by id, the invite form and delete space
      board.ts          threads of a blackboard (most recently active first)
      thread.ts         a thread: title, posts with author and time, reply form (W3)
      inbox.ts          the signed-in agent's own inbox, read-only ("Inbox of <kind> <name>", no acknowledge)
      admins.ts         (W2) admin list, mint form, revoke button per minted admin (none for the bootstrap admin)
      confirm.ts        (W3) confirm section (role alertdialog, `.outcome`) naming space / target / role; takes the focus when it appears (another confirmation is another key) and Escape dismisses it
      shown-once.ts     (W2/W3) invite code or admin token shown once (`.outcome`); takes the focus when it appears; hidden (not removed) while a confirmation is open, so it takes no focus when the confirmation closes
      forms.ts          (W3) invite (space view, leader rights), post (thread view), send-to-topic (space view) forms; submitted through submitForm
    format/
      time.ts           server time → local "YYYY-MM-DD HH:MM" (absolute: a deep-equal poll does not re-render, so a relative time would go stale)
  tests/                bun test, one file per src module
```

Why a separate package (web analysis §3.6): the root `package.json` **is the omp plugin** (`@till-muellers/omp-agent-message-bus`, installed from GitHub), and the GUI is not part of the plugin; the Go module must stay free of `node_modules`. `gui/package.json` adds no new package names (only `typescript` and `@types/bun`, already root devDependencies), so it needs no extra approval beyond PO-10. The root `tsconfig.json` keeps `include: ["extension", "tests"]` (unchanged) and the root `bunfig.toml` sets the test root to `tests` (bun's path arguments are substring filters, so `bun test tests/` alone would match `gui/tests/`), so the root `tsc` and `bun test` never see `gui/`.

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
export type Resource<T> = { status: "idle" } | { status: "loading" } | { status: "ready"; data: T; at: number } | { status: "error"; error: ApiError; last?: T };
export type SessionState = { status: "unknown" } | { status: "signed-out"; error?: string } | { status: "signed-in"; identity: Identity; csrf: string };
export interface AppState {
	session: SessionState;
	route: Route;
	view: Resource<ViewData>;         // the current route's data; replaced on navigation
	shownOnce?: { label: string; secret: string };  // invite code / admin token; cleared on next navigation and sign-out (W2/W3)
	notice?: { message: string; alert: boolean };   // last mutation's outcome (alert: the server's refusal); cleared on next navigation
	confirm?: PendingConfirm;         // W3: {kind: "revoke", space, name, role} | {kind: "delete-space", space}; cleared on navigation
}
// state/actions.ts
export type Action =
	| { type: "session/restored"; identity: Identity; csrf: string } | { type: "session/signed-out"; error?: string }
	| { type: "navigate"; route: Route }
	| { type: "view/loading" } | { type: "view/loaded"; route: Route; data: ViewData; at: number } | { type: "view/failed"; route: Route; error: ApiError }   // loading only from idle; route: answers for a route already left are dropped
	| { type: "secret/shown"; label: string; secret: string } | { type: "notice/shown"; message: string; alert: boolean } | { type: "confirm/open"; confirm: PendingConfirm } | { type: "confirm/close" };
// state/store.ts
export interface Store { getState(): AppState; dispatch(action: Action): void; subscribe(listener: (s: AppState) => void): () => void; }
export function createStore(reducer: (s: AppState, a: Action) => AppState, initial: AppState): Store;
```

- Render loop: `store.subscribe(state => mount(root, render(state)))`; `render` is `views/layout.ts` choosing the view by `state.route.name`. A `view/loaded` whose data is deep-equal to the current data does not notify (no re-render every 5 s).
- Any `401` from any call dispatches `session/signed-out` (session revoked, rotated token, expired cookie: `gui-access.feature:103-140`). `403` shows the error message in place (e.g. a leader opening `#/admins`).
- `view/mount.ts` patches by tag + `key` prop: same tag and key reuse the node and update props/text; an `<input>`/`<textarea>` that has focus keeps its value. This keeps forms usable while polling re-renders (W3).
- Outcomes stay in view (`.outcome` in `style.css`: `position: sticky; top: 0` inside `<main>`): the confirmation, the secret shown once and the notice are the first children of `<main>`, but the button that caused them may be far down a long page. Sticky keeps them at the top of the viewport while the page scrolls; they leave with their state (a secret and a notice on the next navigation, a confirmation on dismiss, confirm or navigation). The confirmation and the secret also carry `autofocus` and `tabindex="-1"`: `mount.ts` focuses them when they appear (a screen reader reads the dialog name or the secret), Tab then reaches the confirmation's buttons, Escape dismisses the confirmation and the focus goes back to the button that opened it. A notice does not take the focus (the operator keeps typing; its `status` and `alert` roles are announced).
- A form clears only when its mutation went through: `submitForm` resets it once the intent resolves true and the form still holds what was submitted, so a refused invite, mint, reply or topic message keeps the typed text next to the server's error, and text typed while the answer was pending is not wiped. The sign-in token field is the exception: it is cleared at once (`takeField`).
- Long unbroken text wraps (`overflow-wrap: break-word` on `body`, `min-width: 0` on the flex paragraphs of the header and the confirmation); a table wider than the window scrolls inside its `.scroll-x` box. At 360 px the page itself never scrolls sideways.

### 3.3 Polling

```ts
// poll/poller.ts
export interface Clock { setTimeout(fn: () => void, ms: number): number; clearTimeout(id: number): void; now(): number; }
export function createPoller(deps: { load(route: Route): Promise<ViewData> | undefined; store: Store; clock: Clock; visible(): boolean; intervalMs?: number }): { start(): void; stop(): void; refreshNow(): void };
```

- Interval 5 s (`web/analysis.md` §3.5), paused while `document.hidden` (`main.ts` calls `refreshNow` on `visibilitychange` to visible), immediate refresh on sign-in, on navigation and after a successful mutation; on errors back off 5 → 10 → 30 s. A background refresh keeps the shown data (no "Loading…"); only the first load of a route shows it. No SSE in milestone 1 (`live-updates.feature` header); `GET /web/api/events` would replace the poller later without touching views.
- Stale loads have one owner each (R16): the poller drops the answer of a superseded request (navigation, `refreshNow`, `stop`: a generation counter) and dispatches `view/loading` on every load without reading the view; the reducer alone decides whether an answer still fits the route (`sameRoute`) and whether a load shows "Loading…" (only from `idle`).
- The GUI never opens `GET /v1/inbox/stream` and never calls `bus_inbox_ack` (`gui-inbox.feature`, `live-updates.feature:63-76`); the inbox view reads `GET /web/api/state` for `maxInboxLimit`, then `GET /web/api/inbox?limit=<maxInboxLimit>`.

---

## 4. Talking to the server

As in `contracts.md` §10 (checked, every call present). Sign-in errors the login view shows verbatim: 401 `invalid token`/`bearer token required`, 429 `rate_limited "too many failed sign-ins; try again later"` (§10.1).

| Call | Request | Response used by |
|---|---|---|
| Sign in | `POST /web/session`, header `Authorization: Bearer <token>`, no body | `{kind, name, space?, csrf}` + `Set-Cookie amb_session` (`Path=/web`) → `session/restored` |
| Restore after reload | `GET /web/session` (§10.1) | same body incl. the same `csrf`; 401 → login view |
| Sign out | `DELETE /web/session` (CSRF-exempt) | `{ok: true}` |
| Space list | `GET /web/api/state` | `{you, spaces: [SpaceSummary + mayManage + invites?], inbox: {waiting}, maxInboxLimit}` → `views/spaces.ts`; `maxInboxLimit` → the inbox read |
| Space | `GET /web/api/state?space=<s>` | `{you, overview, mayManage, invites?, inbox}` → `views/space.ts`; `mayManage` decides the leader controls; `invites` present only for leader-rights sessions (leader of the space or admin) |
| Pending invites (slice 10) | not called: the invite rows with a cancel button come from the space state's `invites` (`[{id, name, expiresAt, invitedBy}]`); the server's `GET /web/api/spaces/<s>/invites` (contracts §10.6) stays for other web clients | section hidden for members |
| Threads | `GET /web/api/spaces/<s>/blackboards/<b>/threads` | `bus_list_threads` result → `views/board.ts` |
| Thread | `GET /web/api/spaces/<s>/threads/<id>` | `bus_read_thread` result → `views/thread.ts` |
| Inbox | `GET /web/api/inbox?limit=` (no `after`: the GUI shows the oldest unacknowledged entries up to the limit) | `[Entry]` → `views/inbox.ts`; `limit` = the state's `maxInboxLimit` |
| Admins (W2) | `GET /web/api/admins`, `POST /web/api/admins {name}`, `POST /web/api/admins/<n>/revoke` | list; `{name, token}` shown once; `{deleted}` |
| Mutations (W3) | `POST …/invites {name, task?}`, `DELETE …/invites/<id>` + `X-CSRF-Token` (→ `{cancelled}`; slice 10, PO-3, `web/gui-invite-cancel.feature:17-19`), `POST …/revoke {name, confirm: "<space>/<name>"}`, `POST …/threads/<id>/posts {body}`, `POST …/topics/<t>/messages {body, critical?}`, `DELETE /web/api/spaces/<s> {confirm: "<s>"}` | per contract; invite code shown once; a `cancelled: false` result shows "the invite was already used, expired or cancelled" and refreshes |

```ts
// api/api-error.ts
export class ApiError extends Error { constructor(readonly status: number, readonly code: string, message: string) { super(message); } }
// api/http.ts
export type FetchFn = (path: string, init: RequestInit) => Promise<Response>;   // main.ts passes the global fetch
export interface Http {
	get<T>(path: string, decode: Decoder<T>): Promise<T>;
	send<T>(method: "POST" | "DELETE", path: string, body: unknown, decode: Decoder<T>): Promise<T>;   // + X-CSRF-Token
	exempt<T>(method: "POST" | "DELETE", path: string, headers: Record<string, string>, decode: Decoder<T>): Promise<T>;   // sign-in/out: no CSRF, no body
}
export function createHttp(deps: { fetchFn: FetchFn; csrf(): string | undefined }): Http;
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

Rules visible in the views: the signed-in agent is shown by kind and name ("admin ops"), never a placeholder (`spaces-and-participants.feature:114-118`); a secret (invite code, admin token) is shown once with "it cannot be shown again" and is dropped from state on the next navigation (`gui-mutations.feature:85-88`, `gui-admins.feature:73-76`), in view and focused when it appears; the confirm dialog names space, target and role, takes the focus, closes on Escape and sends the `confirm` string the server checks (rule "The outcome of an action is in view where the operator acts", `gui-mutations.feature`).

---

## 6. Build output embedded by Go

- **Owner of the Go side: Arch-server** (`server/internal/web/`: `embed.go`, the handlers, `dist/`). Confirmed by message: `GET /web/` → `index.html`, `GET /web/assets/*` → hashed bundles, no SPA fallback, the server never runs bun.
- `gui/scripts/build.ts`:
  1. `Bun.build({ entrypoints: ["src/main.ts", "src/style.css"], outdir: "<out>/assets", target: "browser", minify: true, naming: "[name]-[hash].[ext]", sourcemap: "none" })`;
  2. removes stale `<out>/assets/*` first;
  3. writes `<out>/index.html` from `gui/index.html`, replacing `%MAIN_SCRIPT%` with `/web/assets/main-<hash>.js` and `%STYLE%` with `/web/assets/style-<hash>.css`;
  4. `--out <dir>` (default `../server/internal/web/dist`), used by Docker (§pipeline).
- **Placeholder (contracts.md §10.5).** `server/internal/web/embed.go` uses `//go:embed all:dist`; tracked are only `server/internal/web/dist/.gitkeep` and `server/internal/web/.gitignore` (`dist/*`, `!dist/.gitkeep`). Without a GUI build the server answers `GET /web/` with a built-in fallback page, so `go build`/`go test` work on a fresh checkout without bun, and a local GUI build leaves `git status` clean.
  - The root `.gitignore` line `dist/` (`.gitignore:2`) matches `server/internal/web/dist/` and would keep `.gitkeep` from being added: W1a changes it to `/dist/` (the release output of `scripts/build.py --out dist`).
- **Dev loop.** `bun run --cwd gui build` (re-run after each change; `build.ts` has no watch mode) + rebuild/restart the Go server (embed is compile-time). A dev-only `AMB_WEB_DIR` (CR-G2) was declined by Arch-server (Q-A7 in `QUESTIONS.md`; `contracts.md` §12); no spec needs it.
- **Headers (contracts.md §10.5).** CSP `default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`, `nosniff`, `no-referrer`; `index.html` no-cache, `assets/*` immutable, `/web/session` and `/web/api/*` no-store. The SPA therefore uses no inline script or style, no `<base>`, and submits forms only through `fetch`.

---

## 7. Test approach (bun test)

`bun test` in `gui/` (its own tsconfig; `describe`/`test` from `bun:test`, sentence-style test names as in `tests/*.test.ts`).

| Module | How it is tested |
|---|---|
| `router/routes.ts` | table-driven: hash → route → hash round trip; invalid names → not-found |
| `state/reducer.ts` | table-driven per action; secret cleared on navigate; 401 → signed-out |
| `api/http.ts`, `api/*.ts` | fake `fetchFn` recording method, path, headers (CSRF present on non-GET, absent on GET), body; error envelope → `ApiError` |
| `api/decode*.ts` | valid shapes pass, every missing/wrong field throws `invalid_response`; the state and inbox cases are variants of the wire fixtures below |
| contract (R1) | `tests/api/contract.test.ts` decodes every fixture in `server/internal/web/testdata/` with the production decoder, writes the result back in wire shape and compares it with the fixture, so a renamed, added or dropped field fails (only `you` and the post/send `receipts` are left out on purpose); a fixture without a decoder fails too. The fixtures are marshalled from the real `/web` handlers by `server/internal/server/web_contract_test.go` (times, secrets and invite ids normalized); after a wire change run `cd server && go test ./internal/server -run TestWebContractFixtures -update`, and the Go test fails while they are stale |
| `poll/poller.ts` | fake `Clock`: interval, pause when hidden, backoff, refresh after mutation |
| `views/*.ts` | assert the VNode tree (`h()` output): texts, order, keys, links (`hrefOf`), buttons per kind (no admin nav for a member) |
| `view/plan-patch.ts` | `planPatch` and `samePlace` are pure and tested: reuse in place, insert, remove, keyed moves, tag change, nodes the patcher never rendered |
| `view/mount.ts`, `view/form-field.ts` | not unit-tested against a real DOM (no new dependency such as happy-dom); covered by the smoke procedure (focus kept while polling, token field cleared, no token in storage, focus on a new confirmation or secret and back to the trigger on Escape, a refused submit keeps the input). `mount.ts` only applies `planPatch` and the focus rules. The views test what they hand it: the `autofocus`/`tabindex`/`class` attributes, the Escape handler, the keys; the intents test the went-through result `submitForm` reads. |
| `style.css` | `tests/style.test.ts` pins the rules the views rely on (`.outcome` sticky at the top with a background, `.scroll-x`, wrapping); what they look like is the smoke's job. |
| `scripts/build.ts` | builds into a temp `--out` (in-process and through the command line); asserts `index.html` references an existing `assets/main-*.js` and `assets/style-*.css`; each build test has a 60 s limit (the command-line child 30 s) instead of bun's 5 s default, so a busy machine slows them down instead of failing them |
| fitness | `tests/no-raw-html.test.ts`: no `innerHTML`/`outerHTML`/`insertAdjacentHTML`/`document.write` in `gui/src`; `tests/layout.test.ts` (R4): browser globals only in the four DOM-edge modules (§1), no `state/*` → `views/*` import, every relative import resolves, no import cycle |

Server behaviour (cookie flags, 401/403/405, CSRF binding, loopback rule) is tested in Go by Arch-server's W slices; the GUI tests only the client's side of each contract.

**Smoke procedure (per GUI slice):** throwaway server (`AMB_ADDR=127.0.0.1:<port>`, `AMB_WEB_ADDR=127.0.0.1:<port2>`, `AMB_DB_PATH=<tmp>/x.db`, `AMB_ADMIN_TOKEN=<32..512 visible ASCII chars>`), GUI built into `server/internal/web/dist`, `go run .` in `server/`; open `http://127.0.0.1:<port2>/web/` (the GUI listener `AMB_WEB_ADDR`; the API listener `AMB_ADDR` serves no `/web`); sign in with the admin token; seed data with `POST /v1/tools/<name>` on `AMB_ADDR` (`.omp/AGENTS.md` "Running it locally"); check each view, a 5 s refresh after a seeded change, sign-out, and (W3) every mutation's confirm text; DevTools → Application shows no token in storage, cookie `HttpOnly`. On a page taller than the window (for example 900×500) also check that the confirmation, the invite code and a notice are in view when the button at the bottom is used, that Tab reaches the confirmation's buttons (Enter confirms, Escape cancels and returns the focus), that a refused invite keeps the typed name, and at 360×640 that a 400-character unbroken post does not make the page scroll sideways (`document.documentElement.scrollWidth === innerWidth`).
