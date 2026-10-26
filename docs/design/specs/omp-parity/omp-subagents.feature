Feature: Bus tools in omp subagents: blackboards only
  As a main omp session coordinating over the bus
  I want my subagents to read and write blackboards but nothing else on the bus
  So that only the main session messages, streams mail and manages the space,
  while subagents talk to it through omp's own `write agent://<id>` and `wait`

  # PO ruling (round 2). How omp gives a subagent extension tools (18.12.6):
  #   - every subagent session re-binds the parent's extension modules to its own
  #     ExtensionAPI (sdk.ts:2759-2790, executor.ts:4648-4649) and emits its own
  #     session_start (modes/runtime-init.ts:212), with ctx.agent.kind "sub"
  #     (extensibility/extensions/types.ts:520-537);
  #   - a tool the subagent's extension instance registers is enabled on registration
  #     unless it is defaultInactive or hidden (sdk.ts:4235-4248 at startup,
  #     sdk.ts:5013-5045 for later registrations), even when the agent definition
  #     lists its tools explicitly;
  #   - filterActiveTools only strips the parent-owned `todo` tool
  #     (task/executor.ts:4784-4787, 4817-4821, 4886); it does not strip extension tools;
  #   - restricted subagents (restrictToolNames, e.g. seance) bind no extension tools
  #     (sdk.ts:3546, 5062).
  # So the extension instance in the subagent decides alone what the subagent gets:
  # withholding must happen at registration, not only at activation.

  Background:
    Given a bus with leader "lead" and member "worker" in space "alpha"
    And member "worker" runs in an omp TUI session with the extension loaded and agent credentials
    And member "worker"'s session spawned an omp subagent "0-Explore"

  Rule: A subagent gets exactly the blackboard read/post tools

    Scenario: The subagent's bus tools
      Then the subagent's active tools starting with "bus_" are exactly:
        | tool                 |
        | bus_list_blackboards |
        | bus_list_threads     |
        | bus_read_thread      |
        | bus_create_thread    |
        | bus_add_post         |

    Scenario Outline: The subagent never gets <tool>
      Then the subagent's extension instance does not register "<tool>"
      And "<tool>" is not in the subagent's tool list, not even as a discoverable tool

      Examples:
        | tool                   |
        | bus_create_blackboard  |
        | bus_delete_blackboard  |
        | bus_remove_thread      |
        | bus_subscribe_thread   |
        | bus_unsubscribe_thread |
        | bus_send_direct        |
        | bus_send_topic         |
        | bus_create_topic       |
        | bus_list_topics        |
        | bus_subscribe_topic    |
        | bus_unsubscribe_topic  |
        | bus_close_topic        |
        | bus_inbox_read         |
        | bus_inbox_ack          |
        | bus_wait               |
        | bus_whoami             |
        | bus_list_spaces        |
        | bus_list_participants  |
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

    Scenario: A tool the server adds later is withheld by default
      Given the server registry gains a new tool "bus_something_new"
      When a subagent session starts
      Then the subagent does not get "bus_something_new"

    Scenario: The main session still gets every bus tool
      Then member "worker"'s main session has all server bus tools and "bus_spawn_worker"

    Scenario: A restricted subagent gets no bus tools
      Given member "worker"'s session started a restricted subagent (for example a seance)
      Then that subagent has no tool starting with "bus_"

  Rule: Subagents reach the main session through omp, not through the bus

    Scenario: A subagent reports to its main session with omp's own messaging
      When the subagent "0-Explore" needs to tell the main session something
      Then it writes to "agent://Main" (or its parent's id)
      And the bus is not involved

    Scenario: A subagent has no inbox and no stream
      When leader "lead" sends a direct message to "worker"
      Then only member "worker"'s main session receives it

  Rule: A subagent acts as its root session's bus identity, or not at all

    # Today a subagent's extension instance resolves its own credentials
    # (extension/lib/connection.ts:52-60, extension/lib/credentials.ts:76-83).
    # It sees the accepted invite (keyed by the root session, extension/lib/session-store.ts:6, 46-48)
    # and the stored token, but not the env credentials (deleted from process.env at the
    # parent's load, extension/lib/env.ts:19-22) and not a /bus promote token
    # (in the parent instance's memory only). It could therefore post as another agent.
    # The env and /bus promote rows need seam S15 (analysis.md §2.6). Until then the
    # stop-gap is: register the blackboard tools in a subagent only when its identity
    # comes from the root session's accepted invite (analysis.md, R11).

    Scenario Outline: The subagent uses the same identity as its main session (<how>)
      Given member "worker"'s main session got its credentials <how>
      When the subagent calls bus_add_post on thread 7
      Then the post's author is the main session's bus name

      Examples:
        | how                                  |
        | by accepting an invite               |
        | from AGENT_BUS_URL + AGENT_BUS_TOKEN |
        | from /bus promote                    |
        | from the stored agent token          |

    Scenario: Without credentials in the main session the subagent has no bus tools
      Given member "worker"'s main session has no agent credentials
      Then the subagent has no tool starting with "bus_"

    Scenario: A subagent never falls back to a different stored identity
      Given the main session acts as leader "lead2" from /bus promote
      And the stored agent token belongs to member "worker"
      When the subagent calls bus_add_post
      Then the post's author is "lead2"

  Rule: Posts by a subagent say which subagent wrote them

    # The server records the token's agent as author, so every post of a subagent
    # shows the main session's bus name. The extension adds the attribution itself
    # instead of relying on a convention the model may forget.

    Scenario: A subagent's post starts with an attribution line
      When the subagent "0-Explore" calls bus_add_post on thread 7 with body "tests pass"
      Then the stored post body is "[via subagent 0-Explore]\ntests pass"
      And the post's author is "worker"

    Scenario: A subagent's new thread starts with an attribution line
      When the subagent "0-Explore" calls bus_create_thread with body "plan draft"
      Then the first post's body starts with "[via subagent 0-Explore]"

    Scenario: The main session's posts carry no attribution line
      When member "worker"'s main session calls bus_add_post with body "done"
      Then the stored post body is "done"

    Scenario: The attribution line counts toward the body limit
      When the subagent calls bus_add_post with a body that fits only without the attribution line
      Then the call fails with invalid "body must be 1..65536 bytes"

    Scenario: The main session is not notified of its own subagents' posts
      # Authors never get notifications for their own posts (server/spec/blackboards.feature).
      Given member "worker" subscribed to thread 7
      When the subagent "0-Explore" adds a post to thread 7
      Then member "worker"'s main session gets no inbox entry for it
      And the other subscribers of thread 7 get a notice from "worker" whose body starts with "[via subagent 0-Explore]"

    Scenario: A thread created by a subagent subscribes the main session
      # The server subscribes a thread's creator (bus_create_thread), and the creator is the main identity.
      When the subagent "0-Explore" creates thread 9 on blackboard "plan"
      And "lead" adds a post to thread 9
      Then member "worker"'s main session gets a notice for that post
