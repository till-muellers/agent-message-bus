Feature: Tool descriptions guide the agent on their own
  As an agent that sees only tool names, descriptions and schemas
  I want every bus tool to say what it is for, who may use it and its main pitfall
  So that I use the right tool correctly without reading other documents

  # Proposed fixes from the doc review (analysis.md, section "Doc review findings").
  # Descriptions live in server/internal/tools/*.go (one registry for MCP, RPC and the omp
  # extension) and in extension/lib/spawn-tool.ts for bus_spawn_worker. omp registers extension
  # tools as "discoverable" by default (pi-coding-agent src/extensibility/extensions/types.ts:898;
  # pi-agent-core src/types.ts:1046-1054): their schemas leave the top-level tool list and the
  # model first sees a short summary, so the opening words of a description matter most.

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
        | bus_inbox_ack         | it deletes every entry with id up to upTo, so never acknowledge past what was handled; acknowledging everything lets the next mail notify again |
        | bus_wait              | it blocks until mail arrives or timeoutSeconds (1..300, default 30) pass, returns [] on timeout, filters with from/threadId and does not acknowledge (omp acknowledges for you) |
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
        | bus_create_thread     | the creator is subscribed and gets later posts by others                                  |
        | bus_revoke_member     | an admin may also revoke leaders, the last leader included, and the space is deleted when nobody remains |
        | bus_delete_space      | an admin may delete any space, even one with other leaders                                |

    Scenario: The critical flag says what happens on each kind of client
      Then the "critical" property of bus_send_direct and bus_send_topic reads "Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true."

  Rule: Access lines name admins where they have leader rights

    # Admins are a strict superset of leaders in every space without joining
    # (docs/design/specs/admin/analysis.md §5).

    Scenario Outline: <tool> states its access for admins
      Then the access of <tool> reads "<access>"

      Examples:
        | tool               | access                                       |
        | bus_space_overview | Participants and admins; no side effects.     |
        | bus_invite_member  | Leaders of the space and admins.              |
        | bus_inbox_read     | Leaders, members and admins.                  |
        | bus_wait           | Leaders, members and admins.                  |
        | bus_spawn_worker   | Leaders of the space and admins.              |

  Rule: The extension's own tool is documented like the server's

    Scenario: bus_spawn_worker has an entry in the generated reference
      When "go run ./cmd/tooldoc" renders server/TOOLS.md
      Then the section "omp extension" lists bus_spawn_worker with its arguments space, name, task and cwd and its result {name, space, launcher}

    Scenario: bus_spawn_worker arguments carry the same rules as the server's
      Then the "space" property of bus_spawn_worker reads "Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$."
      And the "task" property reads "Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report."

  Rule: An admin may spawn workers

    # Admins are a strict superset of leaders (docs/design/specs/admin/admin-role.feature) and may
    # invite into every space, so bus_spawn_worker (which invites, then launches) works for
    # an admin session too (docs/design/specs/guidance/session-context.feature, admin rule).

    Scenario: An admin spawns a worker into a space it has not joined
      Given an omp session logged in with only the token of admin "ops"
      And leader "lead" created space "demo"
      When the agent calls bus_spawn_worker with {"space":"demo","name":"worker","task":"audit the board"}
      Then the call returns name "worker", space "demo" and a launcher
      And the worker's first inbox entry is a direct message from "ops" with body "audit the board"

    Scenario: A member still cannot spawn
      Given an omp session that accepted an invite as member "worker" of space "demo"
      When the agent calls bus_spawn_worker with {"space":"demo","name":"helper","task":"x"}
      Then the call fails with forbidden "only leaders may invite members"

  Rule: Core tools stay visible in omp
    # Decided by the PO for the omp-parity BA (docs/design/specs/omp-parity/analysis.md:257-260,360):
    # the main session's essential tools are the five below, everything else stays discoverable,
    # and bus_inbox_ack is not offered to the model in omp. Supersedes this BA's round-1 list.

    Scenario Outline: <tool> is registered as essential
      Given an omp session connected as member "worker" of space "demo"
      Then <tool> is registered with loadMode "essential"

      Examples:
        | tool            |
        | bus_send_direct |
        | bus_send_topic  |
        | bus_wait        |
        | bus_read_thread |
        | bus_add_post    |

    Scenario: The other bus tools stay discoverable
      Given an omp session connected as member "worker" of space "demo"
      Then bus_space_overview, bus_whoami and bus_delete_blackboard are registered without loadMode
      And the guide's "Start" line still names bus_space_overview, so the agent knows to look it up
