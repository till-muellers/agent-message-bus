---
name: agent-message-bus
description: Use when coordinating with other agents over the agent-message-bus through the `bus_*` tools — leading or joining a space, spawning worker agents (`bus_spawn_worker`) or inviting them with invite codes, sending direct or topic messages (including the `all` broadcast and critical interrupts), sharing persistent state on blackboard threads, and handling inbox entries that arrive as `[agent-bus]` messages. Requires the agent-message-bus omp extension with credentials (`/bus login`, `/bus server start`, `/bus promote`, `/bus accept`, or `AGENT_BUS_URL` + `AGENT_BUS_TOKEN`).
---

# Agent message bus

The bus lets agents work together in **spaces**: direct messages, topics (always including `all`), and blackboards with threads and posts. Everything sent to you lands in your **inbox**. The `bus_*` tools, including the extension's own `bus_spawn_worker`, are active only while the session has agent credentials; an admin token alone activates none. If they are missing, ask the user to run `/bus login <url> <token>`, `/bus promote <leader-name>` (after `/bus login <url> <admin-token>` or, for a local server, `/bus server start`) or, for a worker, `/bus accept <url> <code>`. After `/bus logout` every `bus_*` tool is deactivated until the user logs in again. In a **subagent** session, `bus_inbox_read` and `bus_inbox_ack` don't exist and no inbox is streamed: the parent session owns the inbox.

## Who am I

Start with `bus_whoami`. It returns `{kind, name, space?, spaces:[{name, role}]}`.

| `kind` | Can do |
| :--- | :--- |
| `admin` | Only `bus_mint_leader`. Every space tool fails with `forbidden`. Ask the user to run `/bus promote <leader-name>`. |
| `leader` | Create and join any space, then lead it: invite or revoke members, delete it, send to `all`, send critical. Lists all spaces. |
| `member` | Works in exactly one space (`space` in the result). Messaging, topics and blackboards only. |

Your role inside a space (`spaces[].role`) is what most permissions check, not the token kind.

## Tools

| Area | Tools |
| :--- | :--- |
| Identity, spaces | `bus_whoami`, `bus_list_spaces`, `bus_list_participants`, `bus_create_space`, `bus_join_space`, `bus_leave_space`, `bus_delete_space` |
| Members | `bus_spawn_worker` (omp extension), `bus_invite_member`, `bus_revoke_member` |
| Messages, topics | `bus_send_direct`, `bus_send_topic`, `bus_list_topics`, `bus_create_topic`, `bus_subscribe_topic`, `bus_unsubscribe_topic`, `bus_close_topic` |
| Blackboards | `bus_list_blackboards`, `bus_create_blackboard`, `bus_delete_blackboard`, `bus_list_threads`, `bus_create_thread`, `bus_read_thread`, `bus_add_post`, `bus_subscribe_thread`, `bus_unsubscribe_thread`, `bus_remove_thread` |
| Inbox | `bus_inbox_read`, `bus_inbox_ack` |
| Admin | `bus_mint_leader` |

Names of spaces, agents, topics and blackboards must match `^[a-z0-9][a-z0-9_-]{0,62}$`. Bodies are 1..65536 bytes; thread titles are 1..200 characters. Threads are addressed by the integer `threadId`.

The full per-tool reference (arguments, result shapes, error codes) is [`server/TOOLS.md`](https://github.com/till-muellers/agent-message-bus/blob/main/server/TOOLS.md) in the agent-message-bus repo.

## Leader workflow

1. **Space**: `bus_create_space {space}` creates it with its `all` topic and makes you its leader. To help in an existing one, use `bus_list_spaces` and then `bus_join_space`. A space can have several leaders.
2. **Spawn workers**: `bus_spawn_worker {space, name, task?, cwd?}` invites member `name` and starts it as a separate omp session in a new terminal tab (Orca, tmux, zellij, WezTerm, kitty, Windows Terminal; else a new window). The worker joins the space by itself and gets `task` as its first prompt (it travels with the invite); a relative `cwd` is resolved against yours, and it defaults to yours. It returns `{name, space, launcher}`. Give each worker its own name. A name already used by a leader agent fails with `conflict`; `no terminal launcher found for this environment` means the user has to start the worker by hand (next step).
3. **Invite by hand** (another machine, no launcher, or a non-omp client): `bus_invite_member {space, name, task?}` returns `{name, space, code, expires_at}`, a single-use invite code valid for 10 minutes. `task` is the member's first prompt, returned once when the code is accepted; in omp it is sent to the worker as its first message. Hand the code and the bus url (from `/bus status`; no bus tool returns it) to the user, who runs `/bus accept <url> <code>` in the worker's omp session (or starts it with `omp --bus-url <url> --bus-accept <code>`). Other clients redeem it with `POST <url>/v1/invites/accept` and the code as bearer token, which returns `{name, space, token, task?}`. The member token is valid only in that space. Inviting the same name again gives a new code and an additional token.
4. **Topics**: `bus_create_topic` for each workstream. The creator is subscribed automatically; members subscribe themselves. Use `all` only for announcements meant for everyone, because it reaches every participant and nobody can opt out. Only leaders may send to `all`.
5. **Critical**: `critical: true` (leaders only) makes omp **steer** the receiver: it interrupts the running turn. Use it only when the receiver must stop or change course now, such as a wrong direction, a shared resource that is about to break, or an abort. Everything else is normal and arrives as a queued follow-up after the receiver's current turn.
6. **Wind down**: `bus_revoke_member {space, name}` removes a member, its tokens, its pending inbox entries, its blackboards and every unused invite code for that name. It returns `{deleted}`: `false` when there was nothing to remove, so repeating it is safe. Revoking a leader fails with `conflict`. `bus_delete_space` deletes everything and works only once no other leader is in the space. The last leader cannot leave; delete the space instead.

## Member workflow

1. `bus_whoami` to confirm your space, then `bus_list_participants`, `bus_list_topics` and `bus_list_blackboards` to orient.
2. **Inbox**: in an omp session, entries arrive by themselves as messages:

   ```
   [agent-bus] <space> · <where> · from <sender>[ · CRITICAL]
   <body>
   ```

   `<where>` is `direct`, `topic #<t>`, `topic #<t> closed`, `thread <id> "<title>" on <board>`, `thread <id> "<title>" removed`, or `blackboard <board> removed`. The extension acknowledges each entry right after it delivers it, so the entry is already gone from the server when you see it. Act on it or note it; you can't re-read it later. A CRITICAL entry comes from a leader: drop what you're doing and handle it.
3. **Without the extension stream** (other clients): poll `bus_inbox_read {after?, limit?}` (default 100, at most 500, in id order), process the entries, then `bus_inbox_ack {upTo: <highest processed id>}`. Ack deletes every entry with `id <= upTo`, so never ack past what you have processed.
4. **Report** to your leader with `bus_send_direct` or on the workstream topic. Members cannot send to `all` and cannot send critical messages.
5. **Leave** with `bus_leave_space` when done. Leaving deletes your token (later calls fail with `unauthenticated`), your pending inbox entries, your subscriptions and **all blackboards you own**. Before leaving, hand off any state on your boards: post it to a leader's board, or ask a leader to keep it.

## Shared state: blackboards and threads

- Use topics and direct messages for short-lived chatter. Use blackboard threads for anything others must find later, such as plans, decisions, status and results. Thread posts are the **only** history on the bus; inbox entries vanish on ack.
- Any participant can create a blackboard and becomes its owner. The owner or a leader may delete it. When a leader leaves, the leader's boards stay; when a member leaves, the member's boards are deleted.
- `bus_create_thread` creates the thread with its first post and subscribes you. `bus_add_post` notifies every subscriber except the author. `bus_read_thread` returns all posts. `bus_list_threads` sorts threads by most recent activity.
- Subscribe to the threads you depend on and unsubscribe when done. Only the thread creator or a leader may remove a thread.

## Pitfalls

- **One identity per agent.** The server streams an agent's inbox to one session only. A second session with the same identity logs `agent-bus: another session is already connected as <name>; inbox not delivered here` and gets no inbox until the first one ends; it keeps retrying. `/bus promote` applies to the current session only and is not stored, so after an omp restart the user runs it again. Give every worker its own name; a worker that accepted an invite keeps its identity when its session is resumed.
- The `all` topic cannot be subscribed or unsubscribed (`invalid`) or closed (`forbidden`). Other topics: closing notifies the subscribers with `topic closed` and drops their subscriptions. A closed topic disappears from `bus_list_topics`, and its name can be reused.
- Senders never receive their own topic or post notifications. You cannot send a direct message to yourself.
- A member token works only for its own space (`token is not valid for space <name>`).

## Errors

Tool errors read `<code>: <message>`.

| Code | Typical messages | Do |
| :--- | :--- | :--- |
| `invalid` | `<field> is required`, `invalid arguments: …` (unknown field), `<field> must match …`, `body must be 1..65536 bytes`, `title must be 1..200 characters`, `limit must be 1..500` | Fix the arguments and retry. |
| `unauthenticated` | `invalid token`, `invalid or expired invite code` | The token was revoked, or you left or the space was deleted: stop using the bus and tell the user or your leader. An invite code is used up or older than 10 minutes: ask a leader for a new one. Don't retry. |
| `forbidden` | `only leaders may …`, `not a participant of space <s>`, `only the creator or a leader may …`, `admin token cannot act in spaces; …` | You lack the role. Ask a leader, or join first if you are a leader. Don't retry. |
| `not_found` | `space <s> not found`, `topic <t> not found in space <s>`, `participant <p> not in space <s>`, `thread <id> not found`, `blackboard <b> not found` | Re-list (`bus_list_*`) and use the current names or ids. |
| `conflict` | `… already exists`, `topic <t> is already open`, `already a participant of <s>`, `last leader cannot leave; …`, `space <s> has other leaders; …`, `agent <n> is a leader/member agent` | Reuse the existing object or choose another name. |

Never print tokens in chat, topics or posts. Invite codes go only to the user who starts the worker; `bus_spawn_worker` never shows one.
