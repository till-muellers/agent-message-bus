@option @seam-S6
Feature: bus:// URLs in omp's read and write tools (option, not recommended for now)
  As an omp agent
  I want to message and read the bus with the same read/write idiom omp uses for its own agents
  So that "write agent://<id>" and "write bus://<space>/<name>" feel the same

  # omp's native messaging is not a tool of its own: it is `write` to `agent://<id>`
  # (internal-urls/agent-protocol.ts:172-205) and `read agent://<id>`. Schemes live in
  # the process-global InternalUrlRouter (internal-urls/router.ts:93-170); omp's RPC
  # host registers its own schemes there (modes/rpc/host-uris.ts:100-126), and every
  # handler's promptDoc becomes a line in the system prompt (system-prompt.ts:970).
  # The extension API has no call for it: the extension would import
  # "@oh-my-pi/pi-coding-agent/internal-urls" directly. See analysis.md, seam S6.
  # The MCP inbox resource uses bus://_inbox/<agent>[/<space>]
  # (docs/design/specs/mail/mcp-inbox-resource.feature); "_inbox" can never be a space name
  # (names start with [a-z0-9]), so the two never collide.

  Background:
    Given a bus with leader "lead" and members "worker" and "peer" in space "alpha"
    And member "worker" runs in an omp session with the extension loaded and agent credentials

  Rule: The scheme exists only while the session has agent credentials

    Scenario: Logging in registers the scheme and its prompt line
      Then the system prompt lists "bus://" with a one-line description

    Scenario: Logging out removes the scheme
      When the user runs "/bus logout"
      Then reading "bus://alpha" fails with an unknown protocol error

  Rule: Subagents may only use blackboard paths

    # The router is process-global, so a subagent's write/read tools see the scheme too.
    # The PO ruling (subagents: blackboard read/post only) must hold for URLs as well.

    Scenario: A subagent writing to an agent or topic URL is refused
      Given member "worker"'s session spawned an omp subagent
      When the subagent writes to "bus://alpha/peer"
      Then the write fails and tells it to message its main session with "agent://"

    Scenario: A subagent may read and post on a thread URL
      Given member "worker"'s session spawned an omp subagent
      When the subagent writes "done" to "bus://alpha/board/plan/7"
      Then thread 7 has a new post whose body starts with "[via subagent"

  Rule: Writing sends

    Scenario: Writing to an agent sends a direct message
      When member "worker" writes "ready for review" to "bus://alpha/peer"
      Then "peer" receives a direct message "ready for review" from "worker" in space "alpha"
      And the write result is the delivery receipt

    Scenario: Writing to a topic sends a topic message
      When member "worker" writes "build is red" to "bus://alpha/topic/dev"
      Then the subscribers of topic "dev" receive it

    Scenario: Writing to a thread adds a post
      When member "worker" writes "done" to "bus://alpha/board/plan/7"
      Then thread 7 has a new post "done" by "worker"

    Scenario: Critical sends are not possible through a URL
      When leader "lead" writes to "bus://alpha/worker?critical=true"
      Then the write fails and points at bus_send_direct with critical true

  Rule: Reading shows shared state

    Scenario: Reading a space shows its participants, topics and blackboards
      When member "worker" reads "bus://alpha"
      Then it gets the participants with role and connection, the open topics and the blackboards

    Scenario: Reading a blackboard lists its threads
      When member "worker" reads "bus://alpha/board/plan"
      Then it gets the threads, most recently active first

    Scenario: Reading a thread returns the thread as a document
      When member "worker" reads "bus://alpha/board/plan/7"
      Then it gets the thread title and all posts in order
      And line selectors work on the result like on any document
