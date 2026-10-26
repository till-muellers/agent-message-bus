Feature: "You have mail" hint on tool results (notice channel for every client)
  As an agent in any client, omp or MCP or plain HTTP
  I want the next bus tool result after mail arrived to tell me so, once
  So that I learn about mail during my turn without polling and without client push support

  # The first consumer of the edge notice (mail-notice.feature). An edge entry stays
  # "unnoticed" until the agent has been told or has looked:
  #   told   = a hint was attached to one of its tool results, or the inbox stream
  #            (GET /v1/inbox/stream) sent the entry
  #   looked = bus_inbox_read or bus_wait returned that entry
  # The state is stored on the entry (owned by the inbox feature), so it works with
  # the stateless /mcp handler and survives restarts.
  #
  # Where the hint goes:
  #   MCP tools/call    an extra text content block after the tool's own content;
  #                     structuredContent is unchanged
  #   POST /v1/tools    an additive field: {"result": …, "mail": {"waiting": n, "critical": n, "spaces": […]}}
  # Hint text (one line):
  #   [agent-bus] You have mail: <n> waiting in <space>[, <space>…][ (<k> critical)]. Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}.
  # Only successful tool results carry the hint; an error result leaves the edge unnoticed.
  #
  # The omp rule of the design spec ("The extension drops the mail field") is a client
  # rule (extension slice E7), not part of this server spec.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "peer" to space "alpha"

  Rule: The first bus tool call after an edge carries the hint, once

    Scenario: A tool result after new mail carries the hint
      Given the "alpha" mailbox of member "worker" is empty
      And "peer" sent a direct message to "worker" in space "alpha"
      When member "worker" calls MCP tool "bus_list_topics" with its "alpha" token
      Then the result's last text content is "[agent-bus] You have mail: 1 waiting in alpha. Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}."
      And the structured content is the plain tool result

    Scenario: The next call carries no hint
      Given member "worker" got the hint for entry "a1"
      When member "worker" calls MCP tool "bus_whoami"
      Then the result has no hint

    Scenario: More mail before the agent acknowledges adds no hint
      Given member "worker" got the hint for entry "a1"
      And "peer" sent another direct message to "worker" in space "alpha"
      When member "worker" calls MCP tool "bus_whoami"
      Then the result has no hint

    Scenario: After acknowledging everything the next edge hints again
      Given member "worker" got the hint for entry "a1"
      And member "worker" acknowledged its inbox up to the id of entry "a1"
      And "peer" sent a direct message to "worker" in space "alpha"
      When member "worker" calls MCP tool "bus_whoami"
      Then the result carries the hint

  Rule: Looking at the mail counts as being told

    Scenario Outline: <tool> returning the edge entry clears the hint
      Given "peer" sent a direct message to "worker" in space "alpha"
      When member "worker" calls <tool>
      And then member "worker" calls MCP tool "bus_whoami"
      Then neither result carries the hint

      Examples:
        | tool           |
        | bus_inbox_read |
        | bus_wait       |

    Scenario: bus_inbox_ack never carries a hint about the entries it removed
      Given member "worker" got entry "a1" from bus_inbox_read
      When member "worker" acknowledges up to the id of entry "a1"
      Then the result is {"acked": 1} without a hint

  Rule: The web API carries the same hint as a field

    Scenario: POST /v1/tools adds a mail field
      Given "peer" sent a non-critical direct message to "worker" in space "alpha"
      When member "worker" calls POST /v1/tools/bus_whoami with its "alpha" token
      Then the response body is {"result": <whoami>, "mail": {"waiting": 1, "critical": 0, "spaces": ["alpha"]}}

    Scenario: Without pending notice the field is absent
      When member "worker" calls POST /v1/tools/bus_whoami with its "alpha" token
      Then the response body has no "mail" field

  Rule: Scopes and identities

    Scenario: A leader's hint lists every space with waiting mail
      Given leader "lead" also created space "beta"
      And leader "lead" has an empty mailbox
      And entries for "lead" were stored in spaces "alpha" and "beta"
      When leader "lead" calls MCP tool "bus_whoami"
      Then the hint reads "You have mail: 2 waiting in alpha, beta."

    Scenario: An admin gets the hint for its own mailbox
      # Admins are named agents with their own inbox (admin-role.feature). Their scope spans
      # every space they joined, like a leader's.
      Given the bootstrap admin minted admin "ops"
      And admin "ops" joined space "alpha"
      And admin "ops" has an empty mailbox
      And "worker" sent a direct message to "ops" in space "alpha"
      When admin "ops" calls MCP tool "bus_whoami"
      Then the hint reads "You have mail: 1 waiting in alpha."

  Rule: Reading the inbox from the web GUI does not count as looking

    Scenario: The web GUI inbox page leaves the hint pending
      # The web GUI reads an admin's inbox through the inbox service's read-only listing
      # (inbox.Read), not through bus_inbox_read, so it never marks an edge as noticed.
      # The operator's agent session still gets its hint.
      Given the bootstrap admin minted admin "ops"
      And admin "ops" joined space "alpha"
      And "worker" sent a direct message to "ops" in space "alpha"
      And the web GUI showed the inbox page of admin "ops"
      When admin "ops" calls MCP tool "bus_whoami"
      Then the result carries the hint
