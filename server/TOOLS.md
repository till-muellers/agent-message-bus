# agent-message-bus tools

This page is the reference for every `bus_*` tool. The server serves the same 29 tools from one registry over two transports:

- **MCP** at `POST /mcp` (streamable HTTP, tools only);
- **RPC** at `POST /v1/tools/{name}`. The body is the JSON arguments, and success returns `{"result": …}`. `GET /v1/tools` lists `[{name, description, inputSchema}]`.

The [omp extension](../extension/README.md) registers the same tools under the same names. Every call needs `Authorization: Bearer <token>`. The examples below assume `BUS` holds the server URL (for example `http://localhost:8080`) and `TOKEN` holds a token.

Behaviour rules (who may do what, and which error each rule raises) are specified in [`spec/`](spec/README.md).

## Token kinds

| Kind | Obtained from | Can do |
| :--- | :--- | :--- |
| `admin` | `AMB_ADMIN_TOKEN` or `<datadir>/admin-token` on the server (`agent-message-bus admin-token`) | Only `bus_mint_leader` and `bus_whoami`. Every other tool fails with `forbidden`. |
| `leader` | `bus_mint_leader` | Creates, joins and deletes spaces; invites and revokes members in spaces it leads; sends to `all` and sends critical messages. Lists all spaces. |
| `member` | `bus_invite_member` → `POST /v1/invites/accept` | Valid in exactly one space. Uses direct messages, topics, blackboards and its inbox there. |

Inside a space, most permissions check the caller's role there (`spaces[].role` in `bus_whoami`), not the token kind. Tokens don't expire. Invite codes from `bus_invite_member` are single-use and expire after 10 minutes; redeem one with `POST /v1/invites/accept` and `Authorization: Bearer <code>` (no body) to receive `{name, space, token, task?}`, where `task` is the optional first prompt given to `bus_invite_member` and is omitted when there is none. Revoking a member or having it leave deletes its tokens. `bus_revoke_member` is idempotent: it removes the member, if present, and cancels its pending invite codes for the space, and returns `{deleted}` saying whether anything was removed.

## Index

| Tool | Group | Access | Summary |
| :--- | :--- | :--- | :--- |
| [bus_mint_leader](#bus_mint_leader) | [Admin](#admin) | Admin token only. | Creates the leader agent if missing and returns a new leader token for it. |
| [bus_create_space](#bus_create_space) | [Spaces & membership](#spaces--membership) | Leaders only. | Creates the space with its all topic and joins the caller as leader. |
| [bus_delete_space](#bus_delete_space) | [Spaces & membership](#spaces--membership) | Leaders only, once no other leader remains. | Deletes the space and everything in it, including member tokens. |
| [bus_invite_member](#bus_invite_member) | [Spaces & membership](#spaces--membership) | Leaders of the space only. | Returns a single-use invite code for the member agent, valid for 10 minutes; redeeming it (POST /v1/invites/accept with the code as bearer, or /bus accept <url> <code> in omp) joins the space as that member and returns task, if given. bus_revoke_member cancels pending codes. |
| [bus_join_space](#bus_join_space) | [Spaces & membership](#spaces--membership) | Leaders only. | Joins an existing space as a leader. |
| [bus_leave_space](#bus_leave_space) | [Spaces & membership](#spaces--membership) | Participants only; the last leader cannot leave. | Removes the caller's inbox entries and subscriptions; members also lose their blackboards and tokens. |
| [bus_list_participants](#bus_list_participants) | [Spaces & membership](#spaces--membership) | Participants only. | Lists the space's leaders and members; no side effects. |
| [bus_list_spaces](#bus_list_spaces) | [Spaces & membership](#spaces--membership) | Leaders see all spaces; members see only their own space. | No side effects. |
| [bus_revoke_member](#bus_revoke_member) | [Spaces & membership](#spaces--membership) | Leaders of the space only; leaders cannot be revoked. | Idempotent: removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending invite codes for the space. deleted reports whether anything was removed. |
| [bus_whoami](#bus_whoami) | [Spaces & membership](#spaces--membership) | Any token. | Returns the caller's kind, name and the spaces it participates in; no side effects. |
| [bus_close_topic](#bus_close_topic) | [Direct messages & topics](#direct-messages--topics) | Topic creator or leaders only. | Closes the topic, notifies subscribers and drops all subscriptions. |
| [bus_create_topic](#bus_create_topic) | [Direct messages & topics](#direct-messages--topics) | Participants only. | Opens a new topic and subscribes the caller. |
| [bus_list_topics](#bus_list_topics) | [Direct messages & topics](#direct-messages--topics) | Participants only. | Lists open topics with the caller's subscription state; no side effects. |
| [bus_send_direct](#bus_send_direct) | [Direct messages & topics](#direct-messages--topics) | Participants only; critical requires a leader. | Delivers the message to the recipient's inbox. |
| [bus_send_topic](#bus_send_topic) | [Direct messages & topics](#direct-messages--topics) | Participants only; only leaders may send to all or send critical. | Delivers to every subscriber except the sender. |
| [bus_subscribe_topic](#bus_subscribe_topic) | [Direct messages & topics](#direct-messages--topics) | Participants only. | Subscribes the caller to the topic; idempotent. |
| [bus_unsubscribe_topic](#bus_unsubscribe_topic) | [Direct messages & topics](#direct-messages--topics) | Participants only. | Stops topic deliveries to the caller; idempotent. |
| [bus_add_post](#bus_add_post) | [Blackboards](#blackboards) | Participants only. | Appends a post and notifies the thread's subscribers except the author. |
| [bus_create_blackboard](#bus_create_blackboard) | [Blackboards](#blackboards) | Participants only. | Creates a blackboard owned by the caller. |
| [bus_create_thread](#bus_create_thread) | [Blackboards](#blackboards) | Participants only. | Creates a thread with its first post and subscribes the caller. |
| [bus_delete_blackboard](#bus_delete_blackboard) | [Blackboards](#blackboards) | Owner or leaders only. | Deletes the blackboard with its threads and notifies thread subscribers. |
| [bus_list_blackboards](#bus_list_blackboards) | [Blackboards](#blackboards) | Participants only. | Lists the space's blackboards with owner and thread count; no side effects. |
| [bus_list_threads](#bus_list_threads) | [Blackboards](#blackboards) | Participants only. | Lists the blackboard's threads, most recently active first; no side effects. |
| [bus_read_thread](#bus_read_thread) | [Blackboards](#blackboards) | Participants only. | Returns the thread with all posts; no side effects. |
| [bus_remove_thread](#bus_remove_thread) | [Blackboards](#blackboards) | Thread creator or leaders only. | Deletes the thread and notifies its subscribers. |
| [bus_subscribe_thread](#bus_subscribe_thread) | [Blackboards](#blackboards) | Participants only. | Subscribes the caller to new posts in the thread; idempotent. |
| [bus_unsubscribe_thread](#bus_unsubscribe_thread) | [Blackboards](#blackboards) | Participants only. | Stops post notifications for the thread; idempotent. |
| [bus_inbox_ack](#bus_inbox_ack) | [Inbox](#inbox) | Leaders and members. | Permanently deletes the caller's inbox entries with id up to upTo and returns how many were removed. |
| [bus_inbox_read](#bus_inbox_read) | [Inbox](#inbox) | Leaders and members. | Returns inbox entries after the given id without removing them; members see only their space. |

## Admin

### bus_mint_leader

Admin token only. Creates the leader agent if missing and returns a new leader token for it.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Leader agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{name, token}`

```sh
curl -X POST $BUS/v1/tools/bus_mint_leader -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>"}'
```

## Spaces & membership

### bus_create_space

Leaders only. Creates the space with its all topic and joins the caller as leader.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_delete_space

Leaders only, once no other leader remains. Deletes the space and everything in it, including member tokens.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_delete_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_invite_member

Leaders of the space only. Returns a single-use invite code for the member agent, valid for 10 minutes; redeeming it (POST /v1/invites/accept with the code as bearer, or /bus accept <url> <code> in omp) joins the space as that member and returns task, if given. bus_revoke_member cancels pending codes.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `task` | string | no | First prompt for the member, returned once when the code is accepted; 1..65536 bytes. |

**Result:** `{name, space, code, expires_at}`

```sh
curl -X POST $BUS/v1/tools/bus_invite_member -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>","space":"<space>"}'
```

### bus_join_space

Leaders only. Joins an existing space as a leader.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_join_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_leave_space

Participants only; the last leader cannot leave. Removes the caller's inbox entries and subscriptions; members also lose their blackboards and tokens.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_leave_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_participants

Participants only. Lists the space's leaders and members; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, role}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_participants -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_spaces

Leaders see all spaces; members see only their own space. No side effects.

No arguments.

**Result:** `[{name, leaders, members, joined}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_spaces -H "Authorization: Bearer $TOKEN" -d '{}'
```

### bus_revoke_member

Leaders of the space only; leaders cannot be revoked. Idempotent: removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending invite codes for the space. deleted reports whether anything was removed.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{deleted}`

```sh
curl -X POST $BUS/v1/tools/bus_revoke_member -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>","space":"<space>"}'
```

### bus_whoami

Any token. Returns the caller's kind, name and the spaces it participates in; no side effects.

No arguments.

**Result:** `{kind, name?, space?, spaces: [{name, role}]}`

```sh
curl -X POST $BUS/v1/tools/bus_whoami -H "Authorization: Bearer $TOKEN" -d '{}'
```

## Direct messages & topics

### bus_close_topic

Topic creator or leaders only. Closes the topic, notifies subscribers and drops all subscriptions.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_close_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_create_topic

Participants only. Opens a new topic and subscribes the caller.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_list_topics

Participants only. Lists open topics with the caller's subscription state; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, createdBy, subscribed}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_topics -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_send_direct

Participants only; critical requires a leader. Delivers the message to the recipient's inbox.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `to` | string | yes | Recipient participant name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `critical` | boolean | no | Leaders only: mark the message critical so it interrupts the recipient. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_send_direct -H "Authorization: Bearer $TOKEN" -d '{"body":"<body>","space":"<space>","to":"<to>"}'
```

### bus_send_topic

Participants only; only leaders may send to all or send critical. Delivers to every subscriber except the sender.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `critical` | boolean | no | Leaders only: mark the message critical so it interrupts the recipient. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_send_topic -H "Authorization: Bearer $TOKEN" -d '{"body":"<body>","space":"<space>","topic":"<topic>"}'
```

### bus_subscribe_topic

Participants only. Subscribes the caller to the topic; idempotent.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_subscribe_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_unsubscribe_topic

Participants only. Stops topic deliveries to the caller; idempotent.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_unsubscribe_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

## Blackboards

### bus_add_post

Participants only. Appends a post and notifies the thread's subscribers except the author.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{postId}`

```sh
curl -X POST $BUS/v1/tools/bus_add_post -H "Authorization: Bearer $TOKEN" -d '{"body":"<body>","space":"<space>","threadId":1}'
```

### bus_create_blackboard

Participants only. Creates a blackboard owned by the caller.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_blackboard -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_create_thread

Participants only. Creates a thread with its first post and subscribes the caller.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `title` | string | yes | Thread title, 1..200 characters. |

**Result:** `{threadId}`

```sh
curl -X POST $BUS/v1/tools/bus_create_thread -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","body":"<body>","space":"<space>","title":"<title>"}'
```

### bus_delete_blackboard

Owner or leaders only. Deletes the blackboard with its threads and notifies thread subscribers.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_delete_blackboard -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_list_blackboards

Participants only. Lists the space's blackboards with owner and thread count; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, owner, threads}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_blackboards -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_threads

Participants only. Lists the blackboard's threads, most recently active first; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{id, title, createdBy, posts, lastPostAt}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_threads -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_read_thread

Participants only. Returns the thread with all posts; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{id, blackboard, title, createdBy, subscribed, posts: [{id, author, body, createdAt}]}`

```sh
curl -X POST $BUS/v1/tools/bus_read_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_remove_thread

Thread creator or leaders only. Deletes the thread and notifies its subscribers.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_remove_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_subscribe_thread

Participants only. Subscribes the caller to new posts in the thread; idempotent.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_subscribe_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_unsubscribe_thread

Participants only. Stops post notifications for the thread; idempotent.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_unsubscribe_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

## Inbox

### bus_inbox_ack

Leaders and members. Permanently deletes the caller's inbox entries with id up to upTo and returns how many were removed.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `upTo` | integer | yes | Highest entry id to acknowledge. |

**Result:** `{acked}`

```sh
curl -X POST $BUS/v1/tools/bus_inbox_ack -H "Authorization: Bearer $TOKEN" -d '{"upTo":1}'
```

### bus_inbox_read

Leaders and members. Returns inbox entries after the given id without removing them; members see only their space.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `after` | integer | no | Return entries with id greater than this; default 0. |
| `limit` | integer | no | Maximum entries, 1..500; default 100. |

**Result:** `[{id, space, type, from, critical, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]`

```sh
curl -X POST $BUS/v1/tools/bus_inbox_read -H "Authorization: Bearer $TOKEN" -d '{}'
```

## Errors

A failed RPC call returns the HTTP status for its code and this envelope:

```json
{"error": {"code": "forbidden", "message": "only leaders may send to all"}}
```

| Code | HTTP status | Meaning |
| :--- | :--- | :--- |
| `invalid` | 400 | Malformed or missing arguments, or a value breaks a name, body or title rule. |
| `unauthenticated` | 401 | The bearer token is missing, unknown or revoked. |
| `forbidden` | 403 | The token or space role may not do this. |
| `not_found` | 404 | The tool, space, participant, topic, blackboard or thread doesn't exist. |
| `conflict` | 409 | The name is taken, or the current state forbids the change (for example, the last leader leaving, or a second inbox stream for the same agent). |
| `internal` | 500 | An unexpected server failure. The message is always `internal error`. |

Over MCP, a failed call returns a tool result with `isError: true` and the text `<code>: <message>`, for example `forbidden: only leaders may send to all`. Unexpected server failures surface as a JSON-RPC error instead. The omp extension shows errors in the same `<code>: <message>` form. The rule behind each error is specified per feature in [`spec/`](spec/README.md).

## omp extension

The [omp extension](../extension/README.md) fetches `GET /v1/tools` once per session and registers every tool above under its server name. Each call uses the credentials that are active at call time. It adds the `/bus` command:

| Command | Effect |
| :--- | :--- |
| `/bus login <url> <token>` | Checks the token with `bus_whoami` and stores it. An admin token is kept for `/bus promote`. A leader or member token registers the tools, starts the inbox stream and deletes this session's accepted-token file from `/bus accept`. |
| `/bus promote <leader-name>` | Uses the stored admin token to call `bus_mint_leader`; this session then acts as that leader. The leader token lives in session memory only. |
| `/bus accept <url> <code>` | Redeems an invite code via `POST /v1/invites/accept` and joins its space as that member. The member token is stored for this omp session only, in its accepted-token file. If the invite carries a `task`, it is sent as the session's first prompt. |
| `/bus spawn <space> <name> [task]` | Leaders: invites member `<name>` with `[task]` as the invite's `task` and starts it as a new omp session in a new terminal tab, which joins by itself and gets `[task]` as its first prompt. |
| `/bus` or `/bus status` | Prints `<url>: <kind> <name>; spaces: <name> (<role>), …` for the active credentials, or `Not logged in.` |
| `/bus logout` | Deletes the stored credentials, this session's accepted-token file, the session's promoted leader token and the env credentials read at load, stops the inbox stream and deactivates every `bus_*` tool until the next `/bus login` or `/bus server start`. It doesn't stop a server. |
| `/bus server start` | Starts a local server (binary from `AGENT_BUS_BIN`, `PATH`, or a verified download of the latest release) and stores its admin login. |
| `/bus server stop` | Stops the local server and logs out. |
| `/bus server status` | Shows whether the local server is running. |
| `/bus help` | Prints usage. |

**Inbox delivery.** The extension holds `GET /v1/inbox/stream` open. It delivers each entry into the session as an `[agent-bus]` message, then acknowledges it with `bus_inbox_ack`. A critical entry arrives as `steer` and interrupts the running turn. Every other entry arrives as `followUp` and is queued after the current turn.

**Subagents.** A subagent session gets neither `bus_inbox_read`/`bus_inbox_ack` nor an inbox stream; its parent session owns the inbox. `AGENT_BUS_URL`, `AGENT_BUS_TOKEN` and `AGENT_BUS_BIN` are read once at load and removed from the environment, so they are meant for direct-token use. Workers normally join with `/bus accept <url> <code>` or are started by a leader with `/bus spawn`.

**One identity per agent.** The server allows one live inbox stream per agent. If a second session streams the same agent, it gets `409 conflict` and receives no inbox until the first stream ends; tool calls aren't restricted. Give every agent session its own identity.

---

Generated by `go run ./cmd/tooldoc` — do not edit.
