@parked
Feature: PARKED: client-side mail bridge for non-omp agents (agent-message-bus inbox wait|watch)
  As an agent running in a client that ignores MCP push (Claude Code, Codex, Cursor, ...)
  I want a local helper that my client's own hook or monitor runs
  So that mail reaches me, and wakes me where the client allows it, without an MCP push

  # PARKED / FUTURE. PO ruling (round 2): not implemented now; kept for reference
  # only (analysis.md, Appendix A). Not part of the recommended sequence.
  #
  # Idea: trade "the MCP server pushes" for "a local helper started by the client
  # pushes", the same shape as the omp extension.
  #
  # Two client subcommands of the bus binary, using the HTTP API and the
  # credentials AGENT_BUS_URL / AGENT_BUS_TOKEN (or a stored login):
  #   inbox wait  [--timeout <s>]  long-poll with bus_wait (bus-wait.feature);
  #                                on mail: print the entries as "[agent-bus] ..." lines
  #                                to stderr, acknowledge them, exit 2; no mail: exit 0
  #   inbox watch                  hold GET /v1/inbox/stream; print one "[agent-bus] ..."
  #                                line block per entry to stdout and acknowledge it
  # Output format = extension/lib/format.ts, so every client sees the same text.
  #
  # Claude Code wiring (research-clients.md §3.2, §3.3), configured by the user or a plugin:
  #   "Stop"/"SessionStart" command hook {"command": "agent-message-bus inbox wait",
  #     "asyncRewake": true, "timeout": 600}  -> exit 2 wakes an idle session and shows
  #     stderr to Claude as a system reminder
  #   plugin monitor {"command": "agent-message-bus inbox watch"} -> each line reaches
  #     Claude as a notification while the session runs

  Background:
    Given leader "lead" and member "worker" share space "alpha"
    And the environment holds member "worker"'s bus URL and "alpha" token

  Rule: inbox wait returns mail once and acknowledges it

    Scenario: Mail already waiting
      Given entries "a1, a2" for "worker" in space "alpha" are waiting
      When "agent-message-bus inbox wait" runs
      Then it prints entries "a1, a2" in the "[agent-bus]" format to stderr
      And it acknowledges up to entry "a2"
      And it exits with code 2

    Scenario: Mail arrives during the wait
      Given the "alpha" mailbox of member "worker" is empty
      And "agent-message-bus inbox wait --timeout 600" is running
      When leader "lead" sends a direct message "start task 1" to "worker"
      Then the command prints that message to stderr and exits with code 2

    Scenario: No mail before the timeout
      Given the "alpha" mailbox of member "worker" is empty
      When "agent-message-bus inbox wait --timeout 1" runs
      Then it prints nothing and exits with code 0

    Scenario: Waits longer than the server maximum are chained
      When "agent-message-bus inbox wait --timeout 900" runs without mail arriving
      Then it calls bus_wait with timeoutSeconds at most 300 until 900 seconds have passed

    Scenario: A failed acknowledgement still reports the mail
      Given entry "a1" for "worker" in space "alpha" is waiting
      And the acknowledgement fails
      When "agent-message-bus inbox wait" runs
      Then it prints entry "a1" to stderr and exits with code 2
      And it reports the acknowledgement error on stderr

  Rule: inbox watch streams like the omp extension

    Scenario: Each entry becomes one line block and is acknowledged
      When "agent-message-bus inbox watch" runs
      And leader "lead" sends a direct message "hi" to "worker"
      Then stdout gets one "[agent-bus] alpha · direct · from lead" block with body "hi"
      And the entry is acknowledged

    Scenario: Another session already holds the stream
      Given member "worker"'s inbox stream is held by another session
      When "agent-message-bus inbox watch" runs
      Then it reports "another session is already connected as worker" on stderr once
      And it keeps retrying with backoff

  Rule: Missing or rejected credentials fail fast

    Scenario Outline: <case>
      Given <setup>
      When "agent-message-bus inbox wait" runs
      Then it exits with code 1 and reports "<message>" on stderr

      Examples:
        | case            | setup                                  | message                       |
        | no credentials  | no bus URL or token is configured      | no bus credentials            |
        | revoked token   | member "worker"'s token was revoked     | unauthenticated: invalid token |
