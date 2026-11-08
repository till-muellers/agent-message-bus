Feature: Tool descriptions guide the agent on their own
  As an agent that sees only tool names, descriptions and schemas
  I want every bus tool to say what it is for, who may use it and its main pitfall
  So that I use the right tool correctly without reading other documents

  # Descriptions live in server/internal/tools/*.go (one registry for MCP, RPC and the omp
  # extension) and in extension/lib/spawn-tool.ts for bus_spawn_worker. omp registers extension
  # tools as "discoverable": the model first sees a short summary, so the opening words of a
  # description matter most.
  #
  # Descriptions state only shipped behaviour.

  Rule: Descriptions open with the purpose; the access rule follows

    Scenario: Every registry description starts with what the tool does
      When the registry is listed
      Then no tool description starts with an access phrase such as "Participants only", "Leaders only" or "Any token"
      And every description still states its access rule
      And the "Access" column of server/TOOLS.md is filled from a separate access field, not from the first sentence

    Scenario: The extension's spawn tool follows the same shape
      Then the description of bus_spawn_worker starts with "Starts a worker agent"
      And it states "Leaders of the space and admins."

  Rule: Descriptions name the pitfall an agent cannot see from the schema

    # The wording is free and not pinned by tests; the topics are. The guide holds the rules
    # that apply everywhere (inbox order, replying); a description holds only what is specific
    # to its tool.

    Scenario Outline: <tool> says <topic>
      Then the description of <tool> says <topic>

      Examples:
        | tool                  | topic                                                                                  |
        | bus_wait              | it blocks, returns [] on timeout, does not acknowledge, and in omp a queued aside, steer or critical ends it early |
        | bus_inbox_read        | it never blocks and does not acknowledge; omp sessions need not call it                |
        | bus_inbox_ack         | acknowledge only what was handled                                                      |
        | bus_read_thread       | the result is the whole thread; a post notice already carries the new post; post ids differ from inbox entry ids |
        | bus_add_post          | each post wakes every subscriber                                                       |
        | bus_create_thread     | only the creator is subscribed                                                         |
        | bus_create_topic      | nobody else is notified                                                                |
        | bus_create_blackboard | a member's boards are deleted when it leaves or is revoked                             |
        | bus_leave_space       | a member's token and boards are deleted                                                |
        | bus_send_direct       | the recipient must be a participant (not an admin that has not joined) and not the sender |
        | bus_send_topic        | receipts show live or queued                                                           |
        | bus_invite_member     | the code is single-use, expires, and how the user redeems it                           |
        | bus_revoke_member     | an admin may also revoke leaders, the last leader included                             |
        | bus_delete_space      | an admin may delete any space                                                          |
        | bus_spawn_worker      | the task arrives as a direct message; each worker needs its own name                   |

    Scenario: The critical flag says who may set it and how omp recipients see it
      Then the "critical" property of bus_send_direct and bus_send_topic names the leaders and admins and the omp interruption

  Rule: Access lines name admins where they have leader rights

    Scenario Outline: <tool> states its access for admins
      Then the access of <tool> reads "<access>"

      Examples:
        | tool               | access                                    |
        | bus_space_overview | Participants and admins; no side effects. |
        | bus_invite_member  | Leaders of the space and admins.          |
        | bus_inbox_read     | Leaders, members and admins.              |
        | bus_wait           | Leaders, members and admins.              |
        | bus_spawn_worker   | Leaders of the space and admins.          |

  Rule: The name rule is stated where a name is created

    Scenario Outline: <tool> states the name rule on <property>
      Then the "<property>" property of <tool> reads "<description>"

      Examples:
        | tool                  | property   | description                                                  |
        | bus_create_space      | space      | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.             |
        | bus_create_topic      | topic      | Topic name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.             |
        | bus_create_blackboard | blackboard | Blackboard name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.        |
        | bus_invite_member     | name       | Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.      |
        | bus_mint_leader       | name       | Leader agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.      |
        | bus_mint_admin        | name       | Admin agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$.       |

    Scenario Outline: <tool> names an existing <property> without the rule
      # The guide states the rule once under Rules.
      Then the "<property>" property of <tool> reads "<description>"

      Examples:
        | tool                | property   | description                 |
        | bus_join_space      | space      | Space name.                 |
        | bus_send_topic      | topic      | Topic name.                 |
        | bus_create_thread   | blackboard | Blackboard name.            |
        | bus_send_direct     | to         | Recipient participant name. |
        | bus_revoke_member   | name       | Member agent name.          |
        | bus_revoke_admin    | name       | Admin agent name.           |

  Rule: The extension's own tool is documented like the server's

    Scenario: bus_spawn_worker has an entry in the generated reference
      When "go run ./cmd/tooldoc" renders server/TOOLS.md
      Then the section "omp extension" lists bus_spawn_worker with its arguments space, name, task and cwd and its result {name, space, launcher}

    Scenario: bus_spawn_worker arguments carry the same rules as the server's
      Then the "space" property of bus_spawn_worker reads "Space name."
      And the "name" property of bus_spawn_worker reads "Member agent name, matching ^[a-z0-9][a-z0-9_-]{0,62}$."
      And the "task" property reads "Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report."
