Feature: Tool descriptions guide the agent on their own
  As an agent that sees only tool names, descriptions and schemas
  I want every bus tool to say what it is for, who may use it and its main pitfall
  So that I use the right tool correctly without reading other documents

  # Descriptions live in server/internal/tools/*.go (one registry for MCP, RPC and the omp
  # extension) and in extension/lib/spawn-tool.ts for bus_spawn_worker. omp registers extension
  # tools as "discoverable": the model first sees a short summary, so the opening words of a
  # description matter most.
  #
  # Descriptions state only shipped behaviour. A later slice extends them: the
  # bus_inbox_ack notify clause (slice 13a).

  Rule: Descriptions open with the purpose; the access rule follows

    Scenario: Every registry description starts with what the tool does
      When the registry is listed
      Then no tool description starts with an access phrase such as "Participants only", "Leaders only" or "Any token"
      And every description still states its access rule
      And the "Access" column of server/TOOLS.md is filled from a separate access field, not from the first sentence

    Scenario: The extension's spawn tool follows the same shape
      Then the description of bus_spawn_worker starts with "Starts a worker agent"
      And it states "Leaders of the space and admins."

  Rule: Descriptions name the pitfalls an agent cannot see from the schema

    Scenario Outline: <tool> mentions <pitfall>
      Then the description of <tool> mentions that <pitfall>

      Examples:
        | tool                  | pitfall                                                                                  |
        | bus_inbox_read        | omp sessions receive entries automatically and need not poll; it never blocks (use bus_wait to wait) and reading does not acknowledge |
        | bus_wait              | it blocks until mail arrives or timeoutSeconds (1..300, default 30) pass, returns [] on timeout, filters with from/threadId and does not acknowledge (omp acknowledges for you) |
        | bus_inbox_ack         | it deletes every entry with id up to upTo, so never acknowledge past what was handled    |
        | bus_create_topic      | nobody else is notified, so subscribers must be told the topic name                       |
        | bus_create_blackboard | a member's boards are deleted when it leaves or is revoked                               |
        | bus_leave_space       | a member's boards, inbox entries, subscriptions and token are deleted                    |
        | bus_send_direct       | the recipient must be a participant of the same space                                     |
        | bus_spawn_worker      | the task is delivered to the worker as a direct message from the caller (a leader or an admin) |
        | bus_spawn_worker      | each worker needs its own name                                                            |
        | bus_spawn_worker      | a name already used by a leader agent fails with conflict, and without a terminal launcher the leader invites by hand |
        | bus_invite_member     | the code is single-use and expires after 10 minutes; the user redeems it with /bus accept <url> <code> or omp --bus-url <url> --bus-accept <code>, other clients with POST /v1/invites/accept |
        | bus_invite_member     | inviting the same name again gives a new code and an additional token                     |
        | bus_join_space        | a space can have several leaders                                                          |
        | bus_subscribe_topic   | the all topic cannot be subscribed or unsubscribed, because everyone receives it          |
        | bus_close_topic       | the all topic cannot be closed, and a closed topic's name can be reused                   |
        | bus_send_direct       | an agent cannot send a direct message to itself                                           |
        | bus_revoke_member     | an admin may also revoke leaders, the last leader included, and the space is deleted when nobody remains |
        | bus_delete_space      | an admin may delete any space, even one with other leaders                                |
        | bus_create_thread     | the creator is subscribed and gets later posts by others                                  |

    Scenario: The critical flag says what happens on each kind of client
      Then the "critical" property of bus_send_direct and bus_send_topic reads "Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true."

  Rule: Access lines name admins where they have leader rights

    Scenario Outline: <tool> states its access for admins
      Then the access of <tool> reads "<access>"

      Examples:
        | tool              | access                           |
        | bus_invite_member | Leaders of the space and admins. |
        | bus_inbox_read    | Leaders, members and admins.     |
        | bus_wait          | Leaders, members and admins.     |
        | bus_spawn_worker  | Leaders of the space and admins. |

  Rule: The extension's own tool is documented like the server's

    Scenario: bus_spawn_worker has an entry in the generated reference
      When "go run ./cmd/tooldoc" renders server/TOOLS.md
      Then the section "omp extension" lists bus_spawn_worker with its arguments space, name, task and cwd and its result {name, space, launcher}

    Scenario: bus_spawn_worker arguments carry the same rules as the server's
      Then the "space" property of bus_spawn_worker reads "Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$."
      And the "task" property reads "Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report."
