Feature: Bus tools in the main omp session, presented like omp's own messaging tools
  As the main omp session of a bus agent
  I want the core bus tools to be as easy to reach, approve and wait on as omp's native messaging
  So that I use the bus without detours, approval prompts or polling

  # Native reference (omp 18.12.6): messaging is `write agent://<id>` (approval tier
  # "read", internal-urls/agent-protocol.ts:149) plus the essential `wait` tool
  # (tools/wait.ts:91-117). Extension tools default to loadMode "discoverable" (hidden
  # behind xd:// devices or tool search) and approval "exec"
  # (extensibility/extensions/types.ts:898, 904-906); extension/lib/tools.ts:30-43
  # sets neither. Subagents get only the blackboard tools: see omp-subagents.feature.

  Background:
    Given a bus with leader "lead" and members "worker" and "peer" in space "alpha"
    And member "worker" runs in an omp main session with the extension loaded and agent credentials

  Rule: The messaging and blackboard core are top-level tools

    Scenario Outline: <tool> is a top-level tool
      Then the tool "<tool>" is registered with loadMode "essential"

      Examples:
        | tool            |
        | bus_send_direct |
        | bus_send_topic  |
        | bus_wait        |
        | bus_read_thread |
        | bus_add_post    |

    Scenario Outline: <tool> stays discoverable
      Then the tool "<tool>" keeps the default loadMode "discoverable"

      Examples:
        | tool                   |
        | bus_whoami             |
        | bus_list_spaces        |
        | bus_list_participants  |
        | bus_list_topics        |
        | bus_create_topic       |
        | bus_subscribe_topic    |
        | bus_unsubscribe_topic  |
        | bus_close_topic        |
        | bus_list_blackboards   |
        | bus_list_threads       |
        | bus_create_thread      |
        | bus_create_blackboard  |
        | bus_delete_blackboard  |
        | bus_remove_thread      |
        | bus_subscribe_thread   |
        | bus_unsubscribe_thread |
        | bus_inbox_read         |
        | bus_create_space       |
        | bus_join_space         |
        | bus_leave_space        |
        | bus_delete_space       |
        | bus_invite_member      |
        | bus_revoke_member      |
        | bus_mint_leader        |
        | bus_mint_admin         |
        | bus_list_admins        |
        | bus_revoke_admin       |
        | bus_space_overview     |
        | bus_spawn_worker       |

    Scenario: A tool the server adds later is discoverable by default
      Given the server registry gains a new tool "bus_something_new"
      Then the main session registers it with the default loadMode "discoverable"

    Scenario: The model cannot acknowledge inbox entries in omp
      # The extension owns the ack watermark (omp-delivery.feature); a manual ack is
      # cumulative (server/spec/inbox.feature:112) and would delete entries still queued.
      Then the main session has no tool "bus_inbox_ack"

  Rule: Approval tiers follow what a tool does

    # Decided (PO-2, round 6): reads, sends, posts and thread creation are "read",
    # management is "write", bus_spawn_worker is "exec".

    Scenario Outline: <tool> has approval tier "<tier>"
      Then the tool "<tool>" is registered with approval "<tier>"

      Examples:
        | tool                  | tier  |
        | bus_whoami            | read  |
        | bus_list_participants | read  |
        | bus_list_topics       | read  |
        | bus_list_blackboards  | read  |
        | bus_list_threads      | read  |
        | bus_read_thread       | read  |
        | bus_inbox_read        | read  |
        | bus_wait              | read  |
        | bus_send_direct       | read  |
        | bus_send_topic        | read  |
        | bus_add_post          | read  |
        | bus_create_thread     | read  |
        | bus_create_topic      | write |
        | bus_close_topic       | write |
        | bus_create_blackboard | write |
        | bus_delete_blackboard | write |
        | bus_remove_thread     | write |
        | bus_invite_member     | write |
        | bus_revoke_member     | write |
        | bus_create_space      | write |
        | bus_delete_space      | write |
        | bus_space_overview    | read  |
        | bus_list_spaces       | read  |
        | bus_join_space        | write |
        | bus_leave_space       | write |
        | bus_subscribe_topic   | write |
        | bus_unsubscribe_topic | write |
        | bus_subscribe_thread  | write |
        | bus_unsubscribe_thread| write |
        | bus_mint_leader       | write |
        | bus_mint_admin        | write |
        | bus_list_admins       | read  |
        | bus_revoke_admin      | write |
        | bus_spawn_worker      | exec  |

    Scenario: Every registered bus tool has an explicit tier
      Then no bus tool is registered with the extension default tier

    # Sending and posting are "read" because omp classifies `write agent://` messages
    # as "read" (agent-protocol.ts:149). Whether bus sends may skip approval is a PO
    # decision (analysis.md, open questions).

  Rule: Waiting for mail (bus_wait), the one long-poll seam

    # One contract for every client, owned by the mail BA (docs/design/specs/mail/analysis.md §5):
    #   bus_wait {timeoutSeconds 1..300 (default 30), from?, threadId?, after?} -> [entries], [] on timeout.
    # The server tool does not ack. In omp the extension excludes the server tool from
    # registerRemoteTools (like bus_spawn_worker) and registers a local tool with the
    # same name, schema and result. The local tool takes entries from the extension's
    # stream dispatcher, so they are not injected a second time, and acks them.
    # Mirrors omp's IrcBus waiter (irc/bus.ts:187-195).
    # "Queued" below means held in the extension's dispatcher, before pi.sendMessage.
    # An entry already handed to omp (as aside, steer or nextTurn) cannot be taken back
    # through the extension API, so bus_wait never returns it; it reaches the model the
    # normal way.

    Scenario: bus_wait returns the next entry
      Given member "worker" has no undelivered entries
      When member "worker" calls bus_wait
      And "lead" then sends a direct message "go" to "worker"
      Then bus_wait returns an array with that entry
      And the entry is not injected into the session a second time
      And the entry counts as consumed for the ack watermark

    Scenario: bus_wait returns at once with the entries still held by the dispatcher
      Given entries "e1" and "e2" for "worker" are held by the dispatcher and not yet handed to omp
      When member "worker" calls bus_wait
      Then bus_wait returns "e1, e2" immediately
      And neither is handed to omp afterwards

    Scenario: bus_wait does not return entries already handed to omp
      Given entry "e1" was already queued as an aside in omp
      When member "worker" calls bus_wait with timeoutSeconds 5
      Then bus_wait does not return "e1"
      And "e1" reaches the model as an aside

    Scenario: bus_wait can wait for one sender
      When member "worker" calls bus_wait with from "lead"
      And "peer" sends a direct message to "worker"
      And then "lead" sends a direct message to "worker"
      Then bus_wait returns the entry from "lead"
      And the entry from "peer" is delivered the normal way, as an aside

    Scenario: bus_wait can wait for one thread
      When member "worker" calls bus_wait with threadId 7
      And "peer" adds a post to thread 7
      Then bus_wait returns that thread_post entry

    Scenario: bus_wait times out with an empty array
      When member "worker" calls bus_wait with timeoutSeconds 30
      And no matching entry arrives within 30 seconds
      Then bus_wait returns []

    Scenario: bus_wait is cancelled with the turn
      When member "worker" calls bus_wait
      And the user interrupts the turn
      Then bus_wait ends without consuming an entry

    Scenario: Without a live stream bus_wait falls back to the server tool
      Given another session holds member "worker"'s inbox stream
      When member "worker" calls bus_wait with timeoutSeconds 10
      Then the extension forwards the call to the server's bus_wait
      And does not ack what it returns

  Rule: Send results read like omp's delivery receipts

    # Needs the server receipt in presence.feature.

    Scenario Outline: The send result says how the message travelled (<receipt>)
      Given the recipient "peer" is <state>
      When member "worker" sends a direct message to "peer"
      Then the tool result reads "<text>"

      Examples:
        | state                          | receipt | text                                            |
        | connected with an inbox stream | live    | Delivered to peer.                              |
        | not connected                  | queued  | Queued for peer; it is not connected right now. |

    Scenario: A topic send reports how many subscribers got it live
      Given topic "dev" has subscribers "peer" (connected) and "lead" (not connected)
      When member "worker" sends a message to topic "dev"
      Then the tool result reads "Delivered to 2 subscriber(s): peer live, lead queued."

    Scenario: The transcript shows a sent message as an outgoing card
      When member "worker" sends a direct message "done" to "lead"
      Then the tool call renders as an outgoing message card "worker -> lead" in space "alpha" with the quoted body
