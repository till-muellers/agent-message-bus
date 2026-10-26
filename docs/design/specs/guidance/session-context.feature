Feature: Bus context in every model request (omp extension)
  As an omp agent connected to the bus
  I want every model request to tell me who I am on the bus, my role, my space and how to work there
  So that I act correctly from my first turn, without loading any skill, and after compaction

  # Proposed (TODO "Decide whether connecting should inject context" and "Explain member and
  # leader usage"). Replaces the retired skill (analysis.md section 6).
  # Seam: the extension's `context` event handler (omp: "Fired before each LLM call",
  # pi-coding-agent src/extensibility/shared-events.ts:184-196) prepends one custom message,
  # customType "agent-bus-context", to the messages sent to the model. omp sends custom messages
  # as developer-role messages (pi-agent-core src/compaction/messages.ts:214-227). The session
  # file is not changed, so the block is never compacted, duplicated or replayed.
  #
  # Why not before_agent_start's systemPrompt: it fires only for user-authored prompts
  # (agent-session.ts:8177-8206). Turns started by bus entries (an `aside` to an idle session,
  # agent-session.ts:9761-9763, or a critical `steer`) go through #promptAgentInitiatedMessage
  # without that preparation, so a spawned worker that only receives [agent-bus] entries would
  # never see a system-prompt addition.
  #
  # Block layout:
  #   <agent-bus-context>
  #   <identity lines, rendered by the extension>
  #   <main session: the guide markdown from GET /v1/guide (agent-guide.feature)>
  #   <subagent: the subagent lines instead of the guide>
  #   </agent-bus-context>
  # Identity data comes from bus_whoami and, for a member, bus_list_participants of its space.
  # It is cached and refreshed only when the connection restarts or a membership tool succeeds,
  # so the block text, and with it the provider prompt cache, stays stable between changes.

  Background:
    Given a bus at "http://127.0.0.1:8080" with leader "lead" in space "demo"
    And the bus serves the guide "GUIDE"

  Rule: A connected main session gets the block at the head of every model request

    Scenario: A member sees its identity, its space and its leaders
      Given an omp session that accepted an invite as member "worker" of space "demo"
      When the model is called
      Then the first message sent to the model is the agent-bus-context block
      And the block contains "You are member `worker` on the agent message bus at http://127.0.0.1:8080."
      And the block contains "Your space: `demo`. Its leaders: `lead`. Direct messages from a leader or an admin are your orders."
      And the block contains "GUIDE"

    Scenario: A leader sees the spaces it leads
      Given leader "lead" also leads space "beta"
      And an omp session promoted to leader "lead"
      When the model is called
      Then the block contains "You are leader `lead` on the agent message bus at http://127.0.0.1:8080."
      And the block contains "Spaces you lead: `beta`, `demo`."

    Scenario: A leader without spaces is told to create one
      Given the admin minted leader "solo"
      And an omp session promoted to leader "solo"
      When the model is called
      Then the block contains "You lead no space yet: create one with bus_create_space."

    Scenario: The block is sent on turns started by bus entries
      Given an idle omp session in tui mode that accepted an invite as member "worker" of space "demo"
      And no user prompt was ever entered in the session
      When the inbox delivers a direct_message entry from "lead" with body "fix the bug" and it starts a turn
      Then the first message of that model request is the agent-bus-context block

    Scenario: Every model call within a turn carries the block
      Given an omp session connected as member "worker" of space "demo"
      When a turn makes three model calls because of tool calls
      Then each of the three requests starts with the same agent-bus-context block

    Scenario: The block is not stored in the session
      Given an omp session connected as member "worker" of space "demo"
      When the model is called twice
      Then the session file contains no agent-bus-context message
      And compaction never sees the block

  Rule: An admin-token session is a full agent session of that admin
    # PO ruling (round 4; server spec docs/design/specs/admin/): admins are named agents of kind `admin`
    # with their own inbox. They act as leaders in every space without joining; participants can
    # message them only in spaces they joined (participant role `admin`). The bootstrap admin is
    # `admin` (the instance token); more admins come from bus_mint_admin.
    # Q11 (analysis.md), decided: an admin token is the lowest-precedence active credential (after
    # promote, env, accepted and stored agent tokens) and gives a full agent session: tools active,
    # inbox stream, bus_wait; bus_inbox_ack is not offered to the model in omp (omp-parity ruling).
    # Today an admin-only session activates no bus tools (extension/README.md:26,57).

    Background:
      Given admin "ops" was minted with bus_mint_admin
      And the bus has spaces "beta" and "demo"

    Scenario: An admin that joined spaces sees where it can be reached
      Given admin "ops" joined space "demo"
      And an omp session logged in with only the token of admin "ops"
      When the model is called
      Then the block contains "You are admin `ops` on the agent message bus at http://127.0.0.1:8080. You act as a leader in every space without joining it."
      And the block contains "You joined: `demo`. Participants there can message you."
      And the block contains "Spaces: `beta`, `demo`."
      And the block contains "GUIDE"

    Scenario: An admin that joined no space is told it cannot be reached
      Given an omp session logged in with only the token of admin "ops"
      When the model is called
      Then the block contains "You are admin `ops` on the agent message bus at http://127.0.0.1:8080. You act as a leader in every space without joining it."
      And the block contains "You joined no space: nobody can message you until you join one with bus_join_space."

    Scenario: The bootstrap admin is named admin
      Given an omp session logged in with only the instance admin token
      When the model is called
      Then the block contains "You are admin `admin` on the agent message bus"

    Scenario: An admin session has the full agent toolset and its inbox
      Given an omp session logged in with only the token of admin "ops"
      Then the bus tools are active, including bus_mint_admin, bus_list_admins, bus_revoke_admin and bus_spawn_worker
      And bus_inbox_read and bus_wait are registered
      And bus_inbox_ack is not offered to the model
      And the inbox stream of "ops" is opened

    Scenario: Joining or leaving a space refreshes the block
      Given an omp session logged in with only the token of admin "ops"
      When the agent calls bus_join_space with {"space":"demo"} successfully
      And the model is called
      Then the block contains "You joined: `demo`. Participants there can message you."

    Scenario: A promoted leader wins over the stored admin token
      Given an omp session that ran "/bus login <url> <admin-token>" and then "/bus promote lead"
      When the model is called
      Then the block contains "You are leader `lead`"
      And the block does not contain "You are admin"

    @needs-S15
    Scenario: A subagent of an admin session acts as that admin
      Given an omp session logged in with only the token of admin "ops"
      When it starts a subagent and the subagent's model is called
      Then the subagent's block contains "You are a subagent of the main session that acts as admin `ops` on the agent message bus."
      And the subagent's active bus tools are exactly the five blackboard tools

  Rule: The block follows the session's credentials

    Scenario Outline: No active bus credentials, no block (<case>)
      Given an omp session <state>
      When the model is called
      Then no agent-bus-context block is sent

      Examples:
        | case          | state                                                        |
        | not logged in | without bus credentials                                      |
        | after logout  | that ran "/bus logout" after a member login                  |
        | revoked       | whose member token was revoked (unauthenticated on restart)  |

    Scenario: Promoting changes the block on the next model call
      Given an omp session connected as member "worker" of space "demo"
      When the user runs "/bus login <url> <admin-token>" and "/bus promote lead"
      And the model is called
      Then the block contains "You are leader `lead`"
      And the block does not contain "worker"

    Scenario: Session switch and branch rebuild the block from the new session's identity
      Given an omp session connected as member "worker" of space "demo"
      When the user runs "/new" and the new session falls back to stored leader credentials "lead"
      And the model is called
      Then the block contains "You are leader `lead`"

    Scenario Outline: A successful membership tool refreshes the identity (<tool>)
      Given an omp session promoted to leader "lead"
      When the agent calls <tool> successfully
      And the model is called
      Then the block lists the spaces from a fresh bus_whoami

      Examples:
        | tool             |
        | bus_create_space |
        | bus_join_space   |
        | bus_leave_space  |
        | bus_delete_space |

    Scenario: Other tool calls do not refetch the identity
      Given an omp session connected as member "worker" of space "demo"
      When the agent calls bus_send_direct and bus_add_post
      And the model is called
      Then bus_whoami was called once since the connection started

  Rule: Subagents only read and post on blackboards, as their main session

    # PO ruling: subagents get only the blackboard read/post tools; everything else goes
    # through the main interactive session, which they reach with omp's own
    # `write agent://<id>` and `wait`. Today subagents get every bus tool except
    # bus_inbox_read/bus_inbox_ack (extension/lib/tools.ts:16-17, connection.ts:56).
    # Scenarios tagged @needs-S15 rely on omp-parity seam S15 (approved, PO-8): subagents act
    # as the main session's identity in every login mode
    # (docs/design/specs/omp-parity/omp-subagents.feature rule 3). They are implemented with S15
    # (REVIEW.md §11, slice E4 before E8).

    Scenario: A subagent gets exactly the five blackboard tools
      Given an omp session connected as member "worker" of space "demo"
      When it starts a subagent
      Then the subagent's active bus tools are exactly:
        | tool                 |
        | bus_list_blackboards |
        | bus_list_threads     |
        | bus_read_thread      |
        | bus_create_thread    |
        | bus_add_post         |
      And the subagent has no bus_spawn_worker, bus_space_overview, bus_send_direct or bus_inbox_read

    Scenario: The subagent's block explains what it may do and as whom
      Given an omp session connected as member "worker" of space "demo"
      When it starts a subagent and the subagent's model is called
      Then the subagent's block contains "You are a subagent of the main session that acts as member `worker` in space `demo` on the agent message bus."
      And the subagent's block contains "On the bus you may only read and post on blackboards: bus_list_blackboards, bus_list_threads, bus_read_thread, bus_create_thread, bus_add_post."
      And the subagent's block contains "Your threads and posts show `worker` as their author; the extension adds a first line `[via subagent <id>]` to each. They don't notify the main session, and a thread you create subscribes `worker`, so its notifications go to the main session, not to you."
      And the subagent's block contains "You have no inbox. For anything else on the bus (messages, topics, members) ask the main session with `write agent://<id>` (its id is in your Peers list) and `wait` for the answer; tell it about posts it needs to know."
      And the subagent's block does not contain "GUIDE"

    @needs-S15
    Scenario: A subagent of a leader session sees the leader's spaces
      Given leader "lead" also leads space "beta"
      And an omp session promoted to leader "lead"
      When it starts a subagent and the subagent's model is called
      Then the subagent's block contains "You are a subagent of the main session that acts as leader `lead` in spaces `beta`, `demo` on the agent message bus."

  Rule: The block degrades instead of failing the request

    Scenario: The guide cannot be fetched
      Given an omp session connected as member "worker" of space "demo"
      And GET /v1/guide fails
      When the model is called
      Then the block contains the identity lines without the guide
      And the extension logs "agent-bus: guide not loaded: <message>" once

    Scenario: The identity cannot be refreshed
      Given an omp session connected as member "worker" of space "demo"
      And bus_whoami fails with a network error during a refresh
      When the model is called
      Then the block keeps the last known identity
      And the model call is not delayed by a retry
