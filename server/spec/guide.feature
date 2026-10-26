Feature: One agent guide, served by the bus to every client
  As an agent using the bus through omp or any other MCP client
  I want the bus itself to tell me how leaders and members work together
  So that the rules I read always match the server that enforces them, whatever client I use

  # Moved here from docs/design/specs/guidance/agent-guide.feature (rules :22, :36, :53, :60
  # and the first half of :94). The guide is one markdown file embedded in the server binary,
  # server/internal/guide/guide.md (go:embed). It is served:
  #   1. as the MCP `instructions` of /mcp (InitializeResult and the 2026-07-28 DiscoverResult);
  #   2. as GET /v1/guide (text/markdown, bearer auth) for the omp extension's context block;
  #   3. as the section "Working on the bus" of the generated server/TOOLS.md, so humans read
  #      the same text (title line dropped, headings one level deeper).

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"

  Rule: MCP clients receive the guide as server instructions

    Scenario: initialize returns the guide
      When a client connects to /mcp with a leader token and sends initialize
      Then the InitializeResult has instructions equal to the embedded guide

    Scenario: The instructions are the same for every token
      When clients connect to /mcp with a leader token and with a member token
      Then both receive the same instructions

    Scenario: The guide fits the instruction budget of omp's MCP client
      # omp truncates server instructions at 4000 characters (pi-coding-agent src/sdk.ts:1408, :4080-4083).
      Then the embedded guide is at most 4000 characters long

  Rule: The omp extension reads the guide over the web API (GET /v1/guide)

    Scenario: A leader or member token gets the guide as markdown
      When a client sends GET /v1/guide with a member token
      Then the response status is 200
      And the "Content-Type" header is "text/markdown; charset=utf-8"
      And the response body is the embedded guide

    Scenario: The admin token may read the guide
      When a client sends GET /v1/guide with the admin token
      Then the response status is 200

    Scenario: The guide needs a bearer token like every /v1 route
      When a client sends GET /v1/guide without an Authorization header
      Then the response status is 401
      And the call fails with unauthenticated "bearer token required"

  Rule: The generated tool reference embeds the guide

    Scenario: TOOLS.md carries the guide
      When "go run ./cmd/tooldoc" renders server/TOOLS.md
      Then TOOLS.md has a section "Working on the bus" whose body is the embedded guide
      And the golden test fails when TOOLS.md and the guide differ

  Rule: The guide covers both roles end to end

    Scenario Outline: The guide states <topic>
      Then the guide contains a statement that <statement>

      Examples:
        | topic                 | statement                                                                                                  |
        | roles                 | leaders orchestrate and members do the work and report back                                                |
        | first steps           | a newly joined agent calls bus_space_overview                                                              |
        | orders                | a direct message from a leader or an admin is an order and the first one is the member's task              |
        | mail in omp           | in omp, entries arrive by themselves as agent-bus messages and are acknowledged once the agent has seen them |
        | queued mail           | in omp, a normal entry is queued and reaches the agent between tool calls without interrupting it, or starts a turn when it is idle |
        | critical              | only a CRITICAL entry interrupts the running turn; it comes from a leader or an admin and is handled first  |
        | mail elsewhere        | other clients act on the "You have mail" hint, read with bus_inbox_read and acknowledge with bus_inbox_ack only what they handled |
        | waiting for mail      | an agent with nothing to do until mail comes calls bus_wait instead of polling                              |
        | member limits         | members cannot send to all and cannot send critical messages                                               |
        | report fallback       | a member reports to the sender of its task and, if that fails with not_found, to a leader of the space     |
        | when to report        | a member reports to the sender of its task when done, blocked or needing a decision, not on progress       |
        | lasting results       | results others need later go to a blackboard thread, because inbox entries vanish on acknowledgement        |
        | hand-off              | before leaving, a member posts its state to a thread on a leader's board, because its own boards are deleted |
        | who ends membership   | a member leaves only when a leader says so; the leader normally revokes it                                  |
        | spawning              | a leader gives each worker its own name and a task with goal, done criteria and where to report            |
        | all and critical      | all is for announcements to everyone and critical only to stop or redirect someone now                     |
        | wind down             | the last leader cannot leave while others remain and deletes the space instead                              |
        | errors                | each error code and what to do about it                                                                    |
        | secrets               | tokens and invite codes are never posted                                                                   |
        | admins                | admins are named agents (bootstrap admin, more via bus_mint_admin) with every leader right in every space without joining, including inviting and spawning workers |
        | admin superset        | an admin may also delete any space and revoke members and leaders in every space                            |
        | admin reachability    | an admin is reachable only in spaces it joined with bus_join_space, and leaving keeps its authority         |
        | admin tools           | admins manage admins with bus_mint_admin, bus_list_admins and bus_revoke_admin, and leave the work to leaders |
        | subagent tools        | an omp subagent may only list boards and threads, read threads, create threads and add posts                |
        | subagent identity     | a subagent's posts show its main session's bus name with a first line [via subagent <id>] and do not notify the main session |
        | subagent escalation   | a subagent asks its main session for anything else with write agent://<id> and wait                        |

  Rule: The guide never refers to a skill

    Scenario: No skill references remain
      Then the guide does not contain "skill"
