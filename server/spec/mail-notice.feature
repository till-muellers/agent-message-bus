Feature: "You have mail" notice (edge-triggered)
  As a leader or member agent whose client cannot take every message pushed into its session
  I want to be told once that my mailbox has mail, instead of polling it
  So that I read and acknowledge my inbox only when there is something in it

  # Mailbox scope = what one token sees with bus_inbox_read:
  #   leader or admin token -> all of the agent's entries, every space
  #   member token -> the member's entries in the token's space
  # The mailbox is "empty" when the scope holds no unacknowledged entry
  # (bus_inbox_ack deletes entries, see inbox.feature).
  #
  # A notice is due on the edge empty -> non-empty of a scope: the delivery
  # (inbox.Deliver transaction) that stores the first entry into an empty scope.
  # The edge is derived from the stored rows inside that transaction and stored on
  # the entry (columns edge and noticed of the inbox table, migration 0003_inbox_notice),
  # so it survives restarts. Later deliveries into a non-empty scope are not edges. Once
  # the scope is empty again, the next delivery is an edge. An edge entry also records
  # whether the agent has been told or has looked (noticed).
  #
  # A channel that starts listening while the scope holds an unnoticed edge gets
  # one notice at once; afterwards only new edges notify.
  #
  # "listens for notices" / "gets a notice" in this file is the server's notice query
  # (inbox.TakeNotice): it returns what waits in the scope and marks the unnoticed edges
  # noticed. No agent sees a notice yet; the first channel is the tool-result hint
  # (slice 13b, mail-hint.feature), which also adds what the notice tells the agent to do.
  # A notice carries no message bodies: it says how many entries are waiting, how many
  # are critical and in which spaces.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" created space "beta"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "worker" to space "beta"

  Rule: A notice is due only when a scope goes from empty to non-empty

    Scenario: The first entry into an empty mailbox is an edge
      Given the "alpha" mailbox of member "worker" is empty
      And member "worker" listens for notices with its "alpha" token
      When an entry for "worker" in space "alpha" is stored
      Then member "worker" gets exactly one notice for its "alpha" mailbox

    Scenario: More entries while the mailbox is non-empty produce no further notice
      Given member "worker" listens for notices with its "alpha" token
      And an entry for "worker" in space "alpha" was stored and noticed
      When two more entries for "worker" in space "alpha" are stored one after the other
      Then member "worker" gets no further notice

    Scenario: Several entries stored in one transaction produce one notice
      Given the "alpha" mailbox of member "worker" is empty
      And member "worker" listens for notices with its "alpha" token
      When an operation stores three entries for "worker" in space "alpha" and commits
      Then member "worker" gets exactly one notice for its "alpha" mailbox

    Scenario: A failed operation produces no notice
      Given member "worker" listens for notices with its "alpha" token
      When an operation produces an entry for "worker" in space "alpha" and then fails
      Then member "worker" gets no notice

  Rule: Emptying the mailbox re-arms the notice

    Scenario: After a full acknowledgement the next entry notifies again
      Given member "worker" listens for notices with its "alpha" token
      And entries "a1, a2" for "worker" in space "alpha" were stored and noticed once
      And member "worker" acknowledged its inbox with its "alpha" token up to the id of entry "a2"
      When entry "a3" for "worker" in space "alpha" is stored
      Then member "worker" gets exactly one notice for its "alpha" mailbox

    Scenario: A partial acknowledgement does not re-arm the notice
      Given member "worker" listens for notices with its "alpha" token
      And entries "a1, a2" for "worker" in space "alpha" were stored and noticed once
      And member "worker" acknowledged its inbox with its "alpha" token up to the id of entry "a1"
      When entry "a3" for "worker" in space "alpha" is stored
      Then member "worker" gets no further notice

    Scenario: Reading without acknowledging does not re-arm the notice
      Given member "worker" listens for notices with its "alpha" token
      And entry "a1" for "worker" in space "alpha" was stored and noticed
      And member "worker" read its inbox with its "alpha" token
      When entry "a2" for "worker" in space "alpha" is stored
      Then member "worker" gets no further notice

    Scenario: Leaving a space empties that scope
      Given entry "a1" for "worker" in space "alpha" was stored
      And member "worker" left space "alpha" and was invited again
      And member "worker" listens for notices with its new "alpha" token
      When entry "a2" for "worker" in space "alpha" is stored
      Then member "worker" gets exactly one notice for its "alpha" mailbox

  Rule: Scopes are independent

    Scenario: A member's spaces have separate mailboxes
      Given member "worker" listens for notices with its "alpha" token
      And entry "b1" for "worker" in space "beta" is waiting
      When entry "a1" for "worker" in space "alpha" is stored
      Then member "worker" gets exactly one notice for its "alpha" mailbox

    Scenario: An entry in another space does not notify the member's token
      Given member "worker" listens for notices with its "alpha" token
      When entry "b1" for "worker" in space "beta" is stored
      Then member "worker" gets no notice on its "alpha" token

    Scenario: A leader's mailbox spans every space
      Given leader "lead" listens for notices
      And entry "l1" for "lead" in space "alpha" was stored and noticed
      When entry "l2" for "lead" in space "beta" is stored
      Then leader "lead" gets no further notice

    Scenario: One recipient's edge does not notify another
      Given leader "lead" and member "worker" listen for notices
      When an entry for "worker" in space "alpha" is stored
      Then member "worker" gets one notice
      And leader "lead" gets none

  Rule: A channel that starts listening learns the current state once

    Scenario: Listening on a non-empty mailbox notifies at once
      Given entries "a1, a2" for "worker" in space "alpha" are waiting
      When member "worker" starts listening for notices with its "alpha" token
      Then member "worker" gets exactly one notice saying 2 entries are waiting

    Scenario: Listening on an empty mailbox stays quiet
      Given the "alpha" mailbox of member "worker" is empty
      When member "worker" starts listening for notices with its "alpha" token
      Then member "worker" gets no notice

    Scenario: A server restart loses no notice
      Given entry "a1" for "worker" in space "alpha" was stored while no channel listened
      And the server restarted
      When member "worker" starts listening for notices with its "alpha" token
      Then member "worker" gets exactly one notice

  Rule: A notice says what is waiting, not what it says

    Scenario: Notice content
      Given entries from "lead" and "peer" for "worker" in space "alpha" are waiting, one of them critical
      When member "worker" gets a notice for its "alpha" mailbox
      Then the notice names the scope "alpha", the number of waiting entries and that one is critical
      And the notice contains no message body

  # Decided (PO-7, round 6): a critical entry always notifies.
  Rule: A critical entry always notifies, even into a non-empty mailbox

    # Exception to the edge rule (decided, PO-7): without it a leader's critical order
    # would sit silently behind an unacknowledged backlog.

    Scenario: A critical entry into an already noticed mailbox notifies again
      Given member "worker" listens for notices with its "alpha" token
      And entry "a1" for "worker" in space "alpha" was stored and noticed
      When leader "lead" sends a critical direct message to "worker" in space "alpha"
      Then member "worker" gets one more notice, marked critical

  Rule: Admins have mailboxes like leaders and count like any sender

    # Superseded the round-3 "@admin, no inbox" rule. New PO ruling, specified in
    # docs/design/specs/admin/: `admin` is an agent kind. Every admin is a named agent (the
    # bootstrap admin is "admin", minted admins have their own names) with its own
    # inbox. Like a leader's, it spans every space. An admin acts as a leader in every
    # space without joining. It can only be messaged, and it only receives `all`, topic
    # and thread entries, in spaces it joined (participant role "admin"). It subscribes
    # only there. Its entries carry its agent name as the sender.
    #
    # Mailbox scope for an admin token: all of the admin's entries, every space (like a leader).

    Scenario: An admin message into an empty mailbox is an edge
      Given the bootstrap admin minted admin "ops"
      And the "alpha" mailbox of member "worker" is empty
      And member "worker" listens for notices with its "alpha" token
      When admin "ops" sends a direct message to "worker" in space "alpha"
      Then member "worker" gets exactly one notice for its "alpha" mailbox
      And the entry's sender is "ops"

    Scenario: An admin message to all is an edge for every empty recipient scope
      Given the bootstrap admin minted admin "ops"
      And the "alpha" mailbox of member "worker" is empty
      And entry "l1" for "lead" in space "alpha" is waiting
      And member "worker" and leader "lead" listen for notices
      When admin "ops" sends a message to topic "all" in space "alpha"
      Then member "worker" gets exactly one notice for its "alpha" mailbox
      And leader "lead" gets no further notice

    Scenario: A critical admin message follows the critical rule
      Given the bootstrap admin minted admin "ops"
      And entry "a1" for "worker" in space "alpha" was stored and noticed
      When admin "ops" sends a critical direct message to "worker" in space "alpha"
      Then member "worker" gets one more notice, marked critical

    Scenario: A joined admin gets notices for its mailbox
      Given the bootstrap admin minted admin "ops"
      And admin "ops" joined space "alpha"
      And the mailbox of admin "ops" is empty
      And admin "ops" listens for notices
      When member "worker" sends a direct message to "ops" in space "alpha"
      Then admin "ops" gets exactly one notice for its mailbox

    Scenario: An admin's mailbox spans every space it joined
      Given the bootstrap admin minted admin "ops"
      And admin "ops" joined spaces "alpha" and "beta"
      And admin "ops" listens for notices
      And entry "o1" for "ops" in space "alpha" was stored and noticed
      When entry "o2" for "ops" in space "beta" is stored
      Then admin "ops" gets no further notice

    Scenario: An admin that has not joined a space gets no mail from it
      Given the bootstrap admin minted admin "ops"
      And admin "ops" listens for notices
      When leader "lead" sends a message to topic "all" in space "alpha"
      Then admin "ops" gets no notice
      And admin "ops" has no inbox entry
