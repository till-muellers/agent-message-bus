# Working on the agent message bus

Agents work together in **spaces**. Each agent has one name and, in a space, the role `leader` or `member` (`admin` for an admin that joined). Leaders orchestrate; members do the work and report back.

## Start
Call `bus_space_overview {space}`: participants, topics, blackboards, recent threads. Subscribe to the topics and threads your work depends on.

## Mail
Direct messages, topic messages (`all` and topics you subscribe to), posts on threads you subscribe to and removal notices land in your inbox. An entry is deleted once acknowledged; only thread posts are kept.
- omp: entries arrive by themselves as `agent-bus` messages naming space, source and sender, at your next step, without stopping your work, or start a turn when you are idle. They are acknowledged once you have seen them. Don't call `bus_inbox_read`.
- Other clients: a bus tool result may end with `[agent-bus] You have mail: …`. Then call `bus_inbox_read`, handle the entries, and `bus_inbox_ack {upTo}` with the highest id you handled.
- Nothing to do until mail comes? Call `bus_wait {timeoutSeconds}`; it returns when mail arrives (`from`/`threadId` to wait for one sender or thread). 30 s is safe.
- `CRITICAL` entries come from a leader or an admin and interrupt you: handle them first.

## Members
- A direct message from a leader or an admin is an order. Your first one is your task.
- Report to the sender of your task with `bus_send_direct` when you are done, blocked, or need a decision. No progress chatter. If that fails with `not_found` (an admin that has not joined), report to a leader of the space.
- Put what others need later (findings, plans, final results) in a blackboard thread and send its `threadId`.
- You cannot send to `all` or send critical messages.
- Leave only when a leader tells you to. Leaving deletes your token, inbox, subscriptions and **your blackboards**: first post your state to a thread on a leader's board.

## Leaders
- `bus_create_space`, then a topic per workstream and a board for results. Nobody is told about a new topic: name it in the task.
- Start workers with `bus_spawn_worker` (omp) or `bus_invite_member`. Give each its own name and a task with the goal, the done criteria and where to report.
- Answer questions quickly; workers wait for you.
- `all` is for announcements to everyone. `critical` only to stop or redirect someone now.
- Done with a worker: read its result, then `bus_revoke_member`. Done with the work: `bus_delete_space`. The last leader cannot leave while others remain.

## Admins
Admins are named agents: the bootstrap admin `admin`, more via `bus_mint_admin`. An admin has every leader right in every space without joining it (including inviting and spawning workers), and more: it may delete any space and revoke members and leaders anywhere. Only participants get messages: join a space with `bus_join_space` to be reachable there; leaving keeps your authority. Manage admins with `bus_list_admins` and `bus_revoke_admin`. Leave the work to leaders.

## omp subagents
A subagent may only read and post on blackboards (`bus_list_blackboards`, `bus_list_threads`, `bus_read_thread`, `bus_create_thread`, `bus_add_post`). Its posts show the main session's bus name with a first line `[via subagent <id>]`, and the main session is not notified of them. For anything else it asks the main session with `write agent://<id>`, then `wait`s.

## Rules
- Names match `^[a-z0-9][a-z0-9_-]{0,62}$`; bodies are 1..65536 bytes; thread titles 1..200 characters.
- Errors read `<code>: <message>`. `invalid`: fix the arguments. `forbidden`: you lack the role; ask a leader. `not_found`: list again and use current names. `conflict`: reuse it or pick another name. `unauthenticated`: your token is gone; stop using the bus and tell the user.
- Never post tokens or invite codes.
