Feature: Waiting for mail (bus_wait)
  As an agent with nothing else to do until mail arrives
  I want one tool call that returns as soon as matching mail is there
  So that I wait for orders or a reply without a polling loop

  # The ONE long-poll seam of the bus. A single tool contract with two implementations:
  #   - server registry tool (POST /v1/tools/bus_wait, MCP tools/call): for every
  #     non-omp client. It reads the mailbox and never acknowledges.
  #   - omp extension local tool with the same name, schema and result (owned by
  #     docs/design/specs/omp-parity/, omp-tools.feature "bus_wait"). It takes entries from the
  #     extension's stream dispatcher instead of injecting them, and acknowledges them.
  #     The extension does not register the remote bus_wait.
  # This file specifies the contract and the server implementation.
  #
  # Arguments (all optional):
  #   timeoutSeconds  integer 1..300, default 30
  #   after           integer; only entries with id > after (default 0)
  #   from            sender name; only entries from that sender
  #   threadId        integer; only entries of that thread
  # Result: the JSON array of matching unacknowledged entries in the caller's scope,
  # in id order, at most 100 (same shape as bus_inbox_read); [] on timeout.
  # Non-matching entries are left alone (server: stay in the mailbox).
  #
  # Server mechanics: subscribe to the live channel inbox/<agentID> first, then read,
  # then wait, as the inbox stream does (inbox.feature, stream rules).
  # Client tool timeouts cap the usable wait (analysis.md §2): Codex and the Cursor CLI
  # 60 s, Claude Desktop 240 s; the default of 30 s fits all researched clients.


  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" created space "beta"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "worker" to space "beta"
    And leader "lead" invited member "peer" to space "alpha"

  Rule: Mail already waiting returns at once

    Scenario: Unacknowledged entries return without blocking
      Given entries "a1, a2" for "worker" in space "alpha" are waiting
      When member "worker" calls bus_wait with its "alpha" token
      Then the call returns entries "a1, a2" at once
      And the entries stay in the mailbox

    Scenario: after skips entries the agent has already seen
      Given entries "a1, a2" for "worker" in space "alpha" are waiting
      When member "worker" calls bus_wait with its "alpha" token and after the id of entry "a2"
      Then the call blocks

  Rule: A wait ends on the first matching entry of the caller's scope

    Scenario: An entry stored during the wait ends it
      Given the "alpha" mailbox of member "worker" is empty
      And member "worker" is in bus_wait with its "alpha" token and timeoutSeconds 60
      When entry "a1" for "worker" in space "alpha" is stored
      Then the call returns entries "a1"

    Scenario: An entry of another space does not end a member's wait
      Given member "worker" is in bus_wait with its "alpha" token
      When entry "b1" for "worker" in space "beta" is stored
      Then the call keeps waiting

    Scenario: An entry stored between subscribing and reading is not missed
      Given the "alpha" mailbox of member "worker" is empty
      When entry "a1" is stored after member "worker"'s bus_wait subscribed and before it read
      Then the call returns entries "a1"

    Scenario: from waits for one sender
      Given member "worker" is in bus_wait with its "alpha" token and from "lead"
      When "peer" sends a direct message to "worker" in space "alpha"
      And then "lead" sends a direct message "go" to "worker" in space "alpha"
      Then the call returns only the entry from "lead"
      And the entry from "peer" stays in the mailbox

    Scenario: threadId waits for one thread
      Given member "worker" subscribed to thread 7 in space "alpha"
      And member "worker" is in bus_wait with its "alpha" token and threadId 7
      When "peer" adds a post to thread 7
      Then the call returns that thread_post entry

    Scenario: Nothing arrives
      Given the "alpha" mailbox of member "worker" is empty
      When member "worker" calls bus_wait with its "alpha" token and timeoutSeconds 1
      Then the call returns an empty list after about 1 second

  Rule: The server never acknowledges

    Scenario: Returned entries still need bus_inbox_ack
      Given member "worker" called bus_wait and got entry "a1"
      When member "worker" calls bus_wait again without after
      Then the call returns entry "a1" at once
      And it disappears only after member "worker" acknowledges it with bus_inbox_ack

  Rule: Arguments are validated

    Scenario Outline: timeoutSeconds outside 1..300 is rejected (<timeout>)
      When leader "lead" calls bus_wait with timeoutSeconds <timeout>
      Then the call fails with invalid "timeoutSeconds must be 1..300"

      Examples:
        | timeout |
        | 0       |
        | 301     |

  Rule: Admins wait on their own mailbox

    # Admins are named agents with their own inbox (docs/design/specs/admin/admin-messaging.feature).
    # Like a leader's, an admin's scope spans every space, but it only ever holds entries
    # of spaces the admin joined.

    Scenario: A joined admin's wait ends on mail for it
      Given the bootstrap admin minted admin "ops"
      And admin "ops" joined space "alpha"
      And admin "ops" is in bus_wait
      When member "worker" sends a direct message to "ops" in space "alpha"
      Then the call returns that direct_message entry

    Scenario: An admin's wait does not end on a space it has not joined
      Given the bootstrap admin minted admin "ops"
      And admin "ops" is in bus_wait with timeoutSeconds 1
      When leader "lead" sends a message to topic "all" in space "beta"
      Then the call returns an empty list after about 1 second

  Rule: Waiting is bounded by the connection and the server

    Scenario: The wait ends when the client goes away
      Given member "worker" is in bus_wait with timeoutSeconds 300
      When the client closes the connection
      Then the server stops waiting and drops its live subscription

    Scenario: The wait ends when the server shuts down
      Given member "worker" is in bus_wait with timeoutSeconds 300
      When the server cancels all streams
      Then the call returns an empty list
      And graceful shutdown does not wait for the full 300 seconds

    Scenario: A dropped live subscription ends the wait with what is stored
      Given member "worker" is in bus_wait with timeoutSeconds 300
      When the live channel drops the wait's subscription because its buffer is full
      Then the call reads the mailbox once more and returns the matching entries, possibly []

    Scenario: Waiting does not take the inbox stream slot
      Given member "worker" opened the inbox stream with its "alpha" token
      When member "worker" calls bus_wait with its "alpha" token and timeoutSeconds 5
      Then the call is not rejected with conflict

  Rule: The tool is in the registry

    # Final registry after every proposal: 34 server tools (docs/design/specs/REVIEW.md §11 lists
    # the count at each merge step; this slice moves transports.feature to its step's count).

    Scenario: bus_wait is in the registry
      When the admin calls GET /v1/tools
      Then the list contains "bus_wait"
      And bus_inbox_read keeps its arguments "after" and "limit" and never blocks
