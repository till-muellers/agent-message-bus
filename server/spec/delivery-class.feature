Feature: Delivery class of inbox entries
  As a client of the bus (the omp extension, an MCP or HTTP client)
  I want every inbox entry to say how urgently it should reach the agent
  So that I map one small set of classes to my own delivery mechanisms instead of
  special-casing entry types, and new entry types work without client changes

  # Every inbox entry carries a field "class", one of:
  #   interrupt  must reach the agent now, interrupting its running turn
  #   queue      reaches the agent at its next step; wakes an idle agent
  #   deferred   reaches the agent with its next turn; never wakes an idle agent
  # The server decides the class from the entry type and the critical flag
  # (inbox.ClassOf, one table in server/internal/inbox/class.go). Clients never derive it.
  #
  # Mapping today:
  #   any entry with critical true              -> interrupt
  #   direct_message, topic_message, thread_post -> queue
  #   topic_closed, thread_removed, blackboard_removed -> deferred

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "peer" to space "alpha"

  Rule: The server assigns the class when it stores an entry

    Scenario Outline: <type> (critical <critical>) is stored with class <class>
      When an entry of type "<type>" with critical <critical> is stored for "worker" in space "alpha"
      Then member "worker"'s inbox returns it with "class": "<class>"

      Examples:
        | type               | critical | class     |
        | direct_message     | false    | queue     |
        | direct_message     | true     | interrupt |
        | topic_message      | false    | queue     |
        | topic_message      | true     | interrupt |
        | thread_post        | false    | queue     |
        | thread_post        | true     | interrupt |
        | topic_closed       | false    | deferred  |
        | thread_removed     | false    | deferred  |
        | blackboard_removed | false    | deferred  |

    Scenario: The invite task arrives as a queue entry
      Given leader "lead" created an invite code for member "new" in space "alpha" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then member "new" has a "direct_message" entry with "class": "queue"

  Rule: The class is part of the entry shape on every read path

    Scenario Outline: <path> returns the class
      Given "peer" sent a direct message to "worker" in space "alpha"
      When member "worker" reads the entry through <path>
      Then the entry has the field "class" with value "queue"

      Examples:
        | path                     |
        | bus_inbox_read           |
        | GET /v1/inbox/stream     |
