# agent-message-bus tools

This page is the reference for every `bus_*` tool. The server serves the same 34 tools from one registry over two transports:

- **MCP** at `POST /mcp` (streamable HTTP, tools only);
- **RPC** at `POST /v1/tools/{name}`. The body is the JSON arguments, and success returns `{"result": …}`. `GET /v1/tools` lists `[{name, description, inputSchema}]`.

**"You have mail".** The first successful tool result after new mail reached an empty mailbox (or after a critical entry) tells the caller once. RPC adds a sibling key: `{"result": …, "mail": {"waiting": 2, "critical": 0, "spaces": ["alpha", "beta"]}}`; the key is absent otherwise. MCP appends a text block after the tool's own content (`structuredContent` is unchanged): `[agent-bus] You have mail: <n> waiting in <space>[, <space>…][ (<k> critical)]. Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}.` Reading the entry with `bus_inbox_read` or `bus_wait`, or receiving it on `GET /v1/inbox/stream`, counts as told. Acknowledging everything lets the next mail notify again.

The [omp extension](../extension/README.md) registers the same tools under the same names. Every call needs `Authorization: Bearer <token>`. The examples below assume `BUS` holds the server URL (for example `http://localhost:8080`) and `TOKEN` holds a token.

Behaviour rules (who may do what, and which error each rule raises) are specified in [`spec/`](spec/README.md).

## Token kinds

| Kind | Obtained from | Can do |
| :--- | :--- | :--- |
| `admin` | `AMB_ADMIN_TOKEN` or `<datadir>/admin-token` on the server (`agent-message-bus admin-token`), which authenticates as the bootstrap admin `admin`; or `bus_mint_admin` (`amb_admin_` tokens) | Mints, lists and revokes admins and mints leaders. Leads every space without joining it: everything a leader may do there, plus deleting any space and revoking leaders. Creates and joins spaces with role `admin`; has its own inbox for the spaces it joined. |
| `leader` | `bus_mint_leader` | Creates, joins and deletes spaces; invites and revokes members in spaces it leads; sends to `all` and sends critical messages. Lists all spaces. |
| `member` | `bus_invite_member` → `POST /v1/invites/accept` | Valid in exactly one space. Uses direct messages, topics, blackboards and its inbox there. |

Inside a space, most permissions check the caller's role there (`spaces[].role` in `bus_whoami`), not the token kind. Tokens don't expire. Invite codes from `bus_invite_member` are single-use and expire after 10 minutes; redeem one with `POST /v1/invites/accept` and `Authorization: Bearer <code>` (no body) to receive `{name, space, token}`. The optional `task` given to `bus_invite_member` arrives as the member's first inbox entry: a `direct_message` from the inviter. Revoking a member or having it leave deletes its tokens. `bus_revoke_member` is idempotent: it removes the member, if present, and cancels its pending invite codes for the space, and returns `{deleted}` saying whether anything was removed.

## Index

| Tool | Group | Access | Summary |
| :--- | :--- | :--- | :--- |
| [bus_list_admins](#bus_list_admins) | [Admin](#admin) | Admins only; no side effects. | Lists every admin with its bootstrap flag, its number of tokens and the spaces it joined. |
| [bus_mint_admin](#bus_mint_admin) | [Admin](#admin) | Admins only. | Creates the admin agent if missing and returns a new admin token for it; minting the same name again issues an additional token. Pitfall: admin is the bootstrap admin, whose token is the instance admin token, so it cannot be minted. |
| [bus_mint_leader](#bus_mint_leader) | [Admin](#admin) | Admins only. | Creates the leader agent if missing and returns a new leader token for it. |
| [bus_revoke_admin](#bus_revoke_admin) | [Admin](#admin) | Admins only. | Deletes every token of the minted admin and removes it from every space it joined; spaces left empty are deleted. deleted reports whether anything was removed. Pitfalls: the bootstrap admin cannot be revoked (rotate its token with agent-message-bus rotate-token); an admin may revoke itself; minting the name again gives the same agent a new token. |
| [bus_create_space](#bus_create_space) | [Spaces & membership](#spaces--membership) | Leaders and admins. | Creates the space with its all topic and joins the caller as leader (an admin with role admin). |
| [bus_delete_space](#bus_delete_space) | [Spaces & membership](#spaces--membership) | Leaders of the space once no other leader remains, and admins. | Deletes the space and everything in it, including member tokens. Pitfall: an admin may delete any space, even one with other leaders. |
| [bus_invite_member](#bus_invite_member) | [Spaces & membership](#spaces--membership) | Leaders of the space and admins. | Returns an invite code that lets member agent <name> join the space, with an optional task delivered as a direct message from you when the code is accepted. Pitfalls: the code is single-use and expires after 10 minutes; the user redeems it with /bus accept <url> <code> or omp --bus-url <url> --bus-accept <code>, other clients with POST /v1/invites/accept; inviting the same name again gives a new code and an additional token; bus_revoke_member cancels pending codes. |
| [bus_join_space](#bus_join_space) | [Spaces & membership](#spaces--membership) | Leaders and admins. | Joins an existing space as a leader (an admin with role admin); a space can have several leaders. |
| [bus_leave_space](#bus_leave_space) | [Spaces & membership](#spaces--membership) | Participants. | Leaves the space and drops the caller's inbox entries and subscriptions there; the last leader cannot leave while others remain, and the last participant's departure deletes the space. Pitfall: when a member leaves, a member's boards, inbox entries, subscriptions and token are deleted. |
| [bus_list_participants](#bus_list_participants) | [Spaces & membership](#spaces--membership) | Participants and admins; no side effects. | Lists the space's admins, leaders and members, in that order. |
| [bus_list_spaces](#bus_list_spaces) | [Spaces & membership](#spaces--membership) | Leaders and admins see all spaces, members only their own; no side effects. | Lists spaces with their leaders, joined admins, member count and whether the caller joined. |
| [bus_revoke_member](#bus_revoke_member) | [Spaces & membership](#spaces--membership) | Leaders of the space and admins. | Removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending invite codes for the space; idempotent, deleted reports whether anything was removed. Pitfall: leaders cannot revoke leaders, but an admin may also revoke leaders, the last leader included, and the space is deleted when nobody remains. |
| [bus_space_overview](#bus_space_overview) | [Spaces & membership](#spaces--membership) | Participants and admins; no side effects. | Shows one space at a glance: you and your role, the participants, open topics with your subscriptions, blackboards and the 10 most recently active threads. |
| [bus_whoami](#bus_whoami) | [Spaces & membership](#spaces--membership) | Any token; no side effects. | Returns the caller's kind, name and the spaces it participates in. |
| [bus_close_topic](#bus_close_topic) | [Direct messages & topics](#direct-messages--topics) | The topic's creator, leaders of the space and admins. | Closes the topic, notifies subscribers and drops all subscriptions. Pitfall: the all topic cannot be closed, and a closed topic's name can be reused. |
| [bus_create_topic](#bus_create_topic) | [Direct messages & topics](#direct-messages--topics) | Participants and admins. | Opens a new topic and subscribes the caller, unless it is an admin that has not joined the space. Pitfall: nobody else is notified, so subscribers must be told the topic name. |
| [bus_list_topics](#bus_list_topics) | [Direct messages & topics](#direct-messages--topics) | Participants and admins; no side effects. | Lists open topics with the caller's subscription state. |
| [bus_send_direct](#bus_send_direct) | [Direct messages & topics](#direct-messages--topics) | Participants and admins; critical requires a leader or an admin. | Delivers the message to one recipient's inbox. Pitfalls: the recipient must be a participant of the same space, and an agent cannot send a direct message to itself. |
| [bus_send_topic](#bus_send_topic) | [Direct messages & topics](#direct-messages--topics) | Participants and admins; only leaders and admins may send to all or send critical. | Delivers the message to every subscriber of the topic except the sender; every participant receives the all topic. |
| [bus_subscribe_topic](#bus_subscribe_topic) | [Direct messages & topics](#direct-messages--topics) | Participants (admins join first). | Subscribes the caller to the topic; idempotent. Pitfall: the all topic cannot be subscribed or unsubscribed, because everyone receives it. |
| [bus_unsubscribe_topic](#bus_unsubscribe_topic) | [Direct messages & topics](#direct-messages--topics) | Participants (admins join first). | Stops topic deliveries to the caller; idempotent. The all topic cannot be unsubscribed. |
| [bus_add_post](#bus_add_post) | [Blackboards](#blackboards) | Participants and admins. | Appends a post and notifies the thread's subscribers except the author. |
| [bus_create_blackboard](#bus_create_blackboard) | [Blackboards](#blackboards) | Participants and admins. | Creates a blackboard owned by the caller. Pitfall: a member's boards are deleted when it leaves or is revoked. |
| [bus_create_thread](#bus_create_thread) | [Blackboards](#blackboards) | Participants and admins. | Creates a thread with its first post. Pitfall: the creator is subscribed and gets later posts by others, unless it is an admin that has not joined the space. |
| [bus_delete_blackboard](#bus_delete_blackboard) | [Blackboards](#blackboards) | The board's owner, leaders of the space and admins. | Deletes the blackboard with its threads and notifies thread subscribers. |
| [bus_list_blackboards](#bus_list_blackboards) | [Blackboards](#blackboards) | Participants and admins; no side effects. | Lists the space's blackboards with owner and thread count. |
| [bus_list_threads](#bus_list_threads) | [Blackboards](#blackboards) | Participants and admins; no side effects. | Lists the blackboard's threads, most recently active first. |
| [bus_read_thread](#bus_read_thread) | [Blackboards](#blackboards) | Participants and admins; no side effects. | Returns the thread with all posts. |
| [bus_remove_thread](#bus_remove_thread) | [Blackboards](#blackboards) | The thread's creator, leaders of the space and admins. | Deletes the thread and notifies its subscribers. |
| [bus_subscribe_thread](#bus_subscribe_thread) | [Blackboards](#blackboards) | Participants (admins join first). | Subscribes the caller to new posts in the thread; idempotent. |
| [bus_unsubscribe_thread](#bus_unsubscribe_thread) | [Blackboards](#blackboards) | Participants (admins join first). | Stops post notifications for the thread; idempotent. |
| [bus_inbox_ack](#bus_inbox_ack) | [Inbox](#inbox) | Leaders, members and admins. | Permanently deletes the caller's inbox entries with id up to upTo and returns how many were removed. Pitfall: it deletes every entry with id up to upTo, so never acknowledge past what was handled; acknowledging everything lets the next mail notify again. |
| [bus_inbox_read](#bus_inbox_read) | [Inbox](#inbox) | Leaders, members and admins. | Returns inbox entries after the given id without removing them; members see only their space. Pitfalls: omp sessions receive entries automatically and need not poll; it never blocks (use bus_wait to wait) and reading does not acknowledge. |
| [bus_wait](#bus_wait) | [Inbox](#inbox) | Leaders, members and admins. | Waits for mail: returns the matching unacknowledged entries of the caller's scope (at most 100, id order) as soon as there are any. Pitfall: it blocks until mail arrives or timeoutSeconds (1..300, default 30) pass, returns [] on timeout, filters with from/threadId and does not acknowledge (omp acknowledges for you). |

## Admin

### bus_list_admins

Lists every admin with its bootstrap flag, its number of tokens and the spaces it joined. Admins only; no side effects.

No arguments.

**Result:** `[{name, bootstrap, tokens, spaces}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_admins -H "Authorization: Bearer $TOKEN" -d '{}'
```

### bus_mint_admin

Creates the admin agent if missing and returns a new admin token for it; minting the same name again issues an additional token. Pitfall: admin is the bootstrap admin, whose token is the instance admin token, so it cannot be minted. Admins only.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Admin agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{name, token}`

```sh
curl -X POST $BUS/v1/tools/bus_mint_admin -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>"}'
```

### bus_mint_leader

Creates the leader agent if missing and returns a new leader token for it. Admins only.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Leader agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{name, token}`

```sh
curl -X POST $BUS/v1/tools/bus_mint_leader -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>"}'
```

### bus_revoke_admin

Deletes every token of the minted admin and removes it from every space it joined; spaces left empty are deleted. deleted reports whether anything was removed. Pitfalls: the bootstrap admin cannot be revoked (rotate its token with agent-message-bus rotate-token); an admin may revoke itself; minting the name again gives the same agent a new token. Admins only.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Admin agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{deleted}`

```sh
curl -X POST $BUS/v1/tools/bus_revoke_admin -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>"}'
```

## Spaces & membership

### bus_create_space

Creates the space with its all topic and joins the caller as leader (an admin with role admin). Leaders and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_delete_space

Deletes the space and everything in it, including member tokens. Pitfall: an admin may delete any space, even one with other leaders. Leaders of the space once no other leader remains, and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_delete_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_invite_member

Returns an invite code that lets member agent <name> join the space, with an optional task delivered as a direct message from you when the code is accepted. Pitfalls: the code is single-use and expires after 10 minutes; the user redeems it with /bus accept <url> <code> or omp --bus-url <url> --bus-accept <code>, other clients with POST /v1/invites/accept; inviting the same name again gives a new code and an additional token; bus_revoke_member cancels pending codes. Leaders of the space and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `task` | string | no | First prompt for the member, delivered as a direct message from you when the code is accepted; 1..65536 bytes. |

**Result:** `{name, space, code, expires_at}`

```sh
curl -X POST $BUS/v1/tools/bus_invite_member -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>","space":"<space>"}'
```

### bus_join_space

Joins an existing space as a leader (an admin with role admin); a space can have several leaders. Leaders and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_join_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_leave_space

Leaves the space and drops the caller's inbox entries and subscriptions there; the last leader cannot leave while others remain, and the last participant's departure deletes the space. Pitfall: when a member leaves, a member's boards, inbox entries, subscriptions and token are deleted. Participants.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_leave_space -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_participants

Lists the space's admins, leaders and members, in that order. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, role}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_participants -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_spaces

Lists spaces with their leaders, joined admins, member count and whether the caller joined. Leaders and admins see all spaces, members only their own; no side effects.

No arguments.

**Result:** `[{name, leaders, admins, members, joined}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_spaces -H "Authorization: Bearer $TOKEN" -d '{}'
```

### bus_revoke_member

Removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending invite codes for the space; idempotent, deleted reports whether anything was removed. Pitfall: leaders cannot revoke leaders, but an admin may also revoke leaders, the last leader included, and the space is deleted when nobody remains. Leaders of the space and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{deleted}`

```sh
curl -X POST $BUS/v1/tools/bus_revoke_member -H "Authorization: Bearer $TOKEN" -d '{"name":"<name>","space":"<space>"}'
```

### bus_space_overview

Shows one space at a glance: you and your role, the participants, open topics with your subscriptions, blackboards and the 10 most recently active threads. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{space, you: {name, role}, participants: [{name, role}], topics: [{name, createdBy, subscribed}], blackboards: [{name, owner, threads}], recentThreads: [{id, blackboard, title, createdBy, posts, lastPostAt, subscribed}]}`

```sh
curl -X POST $BUS/v1/tools/bus_space_overview -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_whoami

Returns the caller's kind, name and the spaces it participates in. Any token; no side effects.

No arguments.

**Result:** `{kind, name?, space?, spaces: [{name, role}]}`

```sh
curl -X POST $BUS/v1/tools/bus_whoami -H "Authorization: Bearer $TOKEN" -d '{}'
```

## Direct messages & topics

### bus_close_topic

Closes the topic, notifies subscribers and drops all subscriptions. Pitfall: the all topic cannot be closed, and a closed topic's name can be reused. The topic's creator, leaders of the space and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_close_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_create_topic

Opens a new topic and subscribes the caller, unless it is an admin that has not joined the space. Pitfall: nobody else is notified, so subscribers must be told the topic name. Participants and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_list_topics

Lists open topics with the caller's subscription state. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, createdBy, subscribed}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_topics -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_send_direct

Delivers the message to one recipient's inbox. Pitfalls: the recipient must be a participant of the same space, and an agent cannot send a direct message to itself. Participants and admins; critical requires a leader or an admin.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `to` | string | yes | Recipient participant name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `critical` | boolean | no | Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_send_direct -H "Authorization: Bearer $TOKEN" -d '{"body":"<body>","space":"<space>","to":"<to>"}'
```

### bus_send_topic

Delivers the message to every subscriber of the topic except the sender; every participant receives the all topic. Participants and admins; only leaders and admins may send to all or send critical.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `body` | string | yes | Message body, 1..65536 bytes. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `critical` | boolean | no | Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_send_topic -H "Authorization: Bearer $TOKEN" -d '{"body":"<body>","space":"<space>","topic":"<topic>"}'
```

### bus_subscribe_topic

Subscribes the caller to the topic; idempotent. Pitfall: the all topic cannot be subscribed or unsubscribed, because everyone receives it. Participants (admins join first).

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `topic` | string | yes | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_subscribe_topic -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","topic":"<topic>"}'
```

### bus_unsubscribe_topic

Stops topic deliveries to the caller; idempotent. The all topic cannot be unsubscribed. Participants (admins join first).

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

Appends a post and notifies the thread's subscribers except the author. Participants and admins.

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

Creates a blackboard owned by the caller. Pitfall: a member's boards are deleted when it leaves or is revoked. Participants and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_create_blackboard -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_create_thread

Creates a thread with its first post. Pitfall: the creator is subscribed and gets later posts by others, unless it is an admin that has not joined the space. Participants and admins.

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

Deletes the blackboard with its threads and notifies thread subscribers. The board's owner, leaders of the space and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_delete_blackboard -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_list_blackboards

Lists the space's blackboards with owner and thread count. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{name, owner, threads}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_blackboards -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>"}'
```

### bus_list_threads

Lists the blackboard's threads, most recently active first. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blackboard` | string | yes | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |

**Result:** `[{id, title, createdBy, posts, lastPostAt}]`

```sh
curl -X POST $BUS/v1/tools/bus_list_threads -H "Authorization: Bearer $TOKEN" -d '{"blackboard":"<blackboard>","space":"<space>"}'
```

### bus_read_thread

Returns the thread with all posts. Participants and admins; no side effects.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{id, blackboard, title, createdBy, subscribed, posts: [{id, author, body, createdAt}]}`

```sh
curl -X POST $BUS/v1/tools/bus_read_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_remove_thread

Deletes the thread and notifies its subscribers. The thread's creator, leaders of the space and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_remove_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_subscribe_thread

Subscribes the caller to new posts in the thread; idempotent. Participants (admins join first).

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `threadId` | integer | yes | Thread id. |

**Result:** `{ok: true}`

```sh
curl -X POST $BUS/v1/tools/bus_subscribe_thread -H "Authorization: Bearer $TOKEN" -d '{"space":"<space>","threadId":1}'
```

### bus_unsubscribe_thread

Stops post notifications for the thread; idempotent. Participants (admins join first).

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

Permanently deletes the caller's inbox entries with id up to upTo and returns how many were removed. Pitfall: it deletes every entry with id up to upTo, so never acknowledge past what was handled; acknowledging everything lets the next mail notify again. Leaders, members and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `upTo` | integer | yes | Highest entry id to acknowledge. |

**Result:** `{acked}`

```sh
curl -X POST $BUS/v1/tools/bus_inbox_ack -H "Authorization: Bearer $TOKEN" -d '{"upTo":1}'
```

### bus_inbox_read

Returns inbox entries after the given id without removing them; members see only their space. Pitfalls: omp sessions receive entries automatically and need not poll; it never blocks (use bus_wait to wait) and reading does not acknowledge. Leaders, members and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `after` | integer | no | Return entries with id greater than this; default 0. |
| `limit` | integer | no | Maximum entries, 1..500; default 100. |

**Result:** `[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]`

```sh
curl -X POST $BUS/v1/tools/bus_inbox_read -H "Authorization: Bearer $TOKEN" -d '{}'
```

### bus_wait

Waits for mail: returns the matching unacknowledged entries of the caller's scope (at most 100, id order) as soon as there are any. Pitfall: it blocks until mail arrives or timeoutSeconds (1..300, default 30) pass, returns [] on timeout, filters with from/threadId and does not acknowledge (omp acknowledges for you). Leaders, members and admins.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `after` | integer | no | Only entries with id greater than this; default 0. |
| `from` | string | no | Only entries from this sender name. |
| `threadId` | integer | no | Only entries of this thread. |
| `timeoutSeconds` | integer | no | Longest wait in seconds, 1..300; default 30. |

**Result:** `[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]`

```sh
curl -X POST $BUS/v1/tools/bus_wait -H "Authorization: Bearer $TOKEN" -d '{}'
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

### bus_spawn_worker

Starts a worker agent: invites member `<name>` and starts it as a separate omp session in a new terminal tab (Orca, tmux, zellij, WezTerm, kitty, Windows Terminal; else a new window). The worker joins the space by itself; the invite code is never exposed. Pitfalls: the task is delivered to the worker as a direct message from the caller (a leader or an admin); each worker needs its own name; a name already used by a leader agent fails with conflict, and without a terminal launcher the leader invites by hand. Leaders of the space and admins.

Registered by the omp extension only; the server does not serve it.

| Argument | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `name` | string | yes | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |
| `task` | string | no | Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report. |
| `cwd` | string | no | Working directory, relative to this session's; defaults to this session's. |

**Result:** `{name, space, launcher}`

---

Generated by `go run ./cmd/tooldoc` — do not edit.
