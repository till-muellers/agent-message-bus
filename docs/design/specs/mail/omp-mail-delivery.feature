Feature: The mail notice and acknowledgement in omp sessions (extension)
  As an agent running in omp with the agent-bus extension
  I want every bus entry to reach my context exactly once, without extra notices
  So that I react to mail promptly and lose nothing

  # PROPOSAL (docs/design/specs/mail/analysis.md §4). Not implemented.
  #
  # PO ruling (round 2): in omp only critical entries steer; every other entry is
  # queued with pi.sendMessage(..., { deliverAs: "aside" }). That delivery, the wake
  # rules per mode and the ack-on-consumption design (ack when the injected message
  # has reached the model's context) belong to docs/design/specs/omp-parity/omp-delivery.feature.
  # This file only pins how they interact with the mail notice (mail-notice.feature)
  # and with the cumulative ack (bus_inbox_ack deletes every entry with id <= upTo).
  #
  # Edge semantics stay a server fact: an entry is an edge when it is stored into a
  # scope with no unacknowledged entry. Acking on consumption keeps entries in the
  # mailbox from arrival on the stream until they reach the context, so fewer entries
  # are edges. Nothing in omp consumes edges, so omp behaviour does not change.

  Background:
    Given leader "lead" and member "worker" share space "alpha"
    And member "worker" runs an omp session in "tui" mode with the extension connected

  Rule: omp shows entries, never "you have mail" notices

    Scenario: An entry into an empty mailbox is injected in full, not as a notice
      Given the "alpha" mailbox of member "worker" is empty
      When leader "lead" sends a direct message "start task 1" to "worker"
      Then the session receives one "[agent-bus]" message containing "start task 1"
      And the session receives no "you have mail" notice

    Scenario: The extension does not ask the server for notices
      When the extension opens the inbox stream
      Then it requests entries, as today
      And no notice channel is opened for this agent

  Rule: The acknowledgement is a contiguous watermark of consumed entries

    # Ack is cumulative. A critical entry steers ahead of asides already queued
    # (omp-delivery.feature), so the extension consumes entries out of id order.
    # It acks up to the highest id at which every entry it has received with a
    # lower or equal id has reached the context.

    Scenario: Entries consumed in order are acknowledged one by one
      Given entries "e1" and "e2" arrive as asides
      When "e1" reaches the context and then "e2" reaches the context
      Then the extension acknowledges up to "e1" and then up to "e2"

    Scenario: A critical entry consumed first does not acknowledge earlier asides
      Given aside "e1" is queued but has not reached the context
      When critical entry "e2" arrives and steers into the context first
      Then the extension acknowledges nothing yet
      And when "e1" reaches the context the extension acknowledges up to "e2"

    Scenario: A session ending with an aside still queued loses nothing
      Given aside "e1" is queued and critical entry "e2" has reached the context
      When the session shuts down before "e1" reaches the context
      Then entries "e1" and "e2" stay in the mailbox
      And the next stream of member "worker" delivers "e1" and "e2" again

    Scenario: A replayed entry that already reached the context is shown again
      Given critical entry "e2" reached the context before the session ended without an acknowledgement
      When the next session of member "worker" receives "e1" and "e2" from the backlog
      Then both are injected again
      # Accepted duplicate: the server keeps no per-entry "seen" state. Rare (needs a
      # crash between consumption and ack), and duplicates are safer than loss.

  Rule: Edges follow the stored rows, whatever the ack timing

    Scenario: An entry stored while an earlier one waits for consumption is not an edge
      Given entry "e1" for "worker" was delivered to the session but has not reached the context
      When entry "e2" for "worker" in space "alpha" is stored
      Then "e2" is not an edge for the "alpha" scope

    Scenario: After the watermark covers everything, the next entry is an edge again
      Given every entry of member "worker" in space "alpha" has been acknowledged
      When entry "e3" for "worker" in space "alpha" is stored
      Then "e3" is an edge for the "alpha" scope

    Scenario: An entry left unacknowledged in print mode keeps the mailbox non-empty
      # One-shot idle rule (omp-delivery.feature): an entry for an idle print/json session
      # is queued for the next turn and counts as consumed only once it is part of a model
      # request. After the one-shot run has ended, no further request comes.
      Given member "worker" runs omp in "print" mode and its one-shot run has ended
      And entry "e1" arrived and was queued for a next turn that never comes
      When entry "e2" for "worker" in space "alpha" is stored
      Then "e2" is not an edge
      And both entries are replayed to the next session

  Rule: bus_wait in omp is the extension's local tool

    Scenario: The remote bus_wait is not registered in omp
      When the extension registers the server's tools
      Then it registers its own bus_wait with the server's schema instead of the remote one
      And an entry returned by bus_wait counts as consumed for the watermark

  Rule: Subagents stay without mail

    Scenario: A subagent gets neither entries, notices nor bus_wait
      Given member "worker"'s session spawned a subagent
      When leader "lead" sends a direct message to "worker"
      Then only the parent session receives it
      And the subagent has no bus_wait tool
