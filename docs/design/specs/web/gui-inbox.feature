Feature: The signed-in admin's inbox in the web GUI
  As a human operator of a bus
  I want to see the inbox of the admin I signed in with
  So that I see what was sent to me without opening a terminal

  # Admins have their own inbox (docs/design/specs/admin/admin-messaging.feature): an
  # admin is a named agent of kind "admin", and an admin's inbox holds the
  # entries of the spaces it joined.
  #
  # The web GUI shows that inbox **read-only**:
  #   - it never acknowledges, so it never deletes an entry that the same
  #     admin's omp session still has to handle (bus_inbox_ack deletes);
  #   - it never opens the inbox stream, so it never takes the admin's stream
  #     slot and never 409s the operator's own session.
  #   - it reads through the inbox service's read-only listing, not through
  #     bus_inbox_read, so it never marks a mail edge as noticed and the operator's
  #     agent session still gets its "You have mail" hint (docs/design/specs/mail/mail-hint.feature).
  # The web GUI therefore shows the entries but cannot clear them; clearing is
  # the agent session's job.
  #
  # Route: GET /web/api/inbox — the signed-in admin's own inbox, JSON.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted admin "ops"
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And admin "ops" joined space "demo"

  Rule: The operator sees the inbox of the admin it signed in with

    Scenario: The inbox page lists the entries of the signed-in admin
      Given member "worker" sent direct message "need a decision" to "ops" in space "demo"
      And the operator signed in with the token of admin "ops"
      When the operator opens the inbox page
      Then the page lists the entry "need a decision" from "worker" in space "demo"

    Scenario: The inbox of another admin is not shown
      Given member "worker" sent direct message "need a decision" to "ops" in space "demo"
      And the operator signed in with the instance admin token
      When the operator opens the inbox page
      Then the page shows no entry

    Scenario: The inbox page shows no entry of a space the admin did not join
      Given leader "lead" created space "other"
      And member "worker" sent direct message "hello" to "ops" in space "demo"
      And leader "lead" tried to send direct message "x" to "ops" in space "other"
      When the operator signed in with the token of admin "ops" opens the inbox page
      Then that send failed with not_found "participant ops not in space other"
      And the page lists only the entry "hello" from space "demo"

    Scenario: The inbox page names the admin it belongs to
      Given the operator signed in with the token of admin "ops"
      When the operator opens the inbox page
      Then the page shows the inbox of admin "ops"

  Rule: The web GUI only reads the inbox

    Scenario: The inbox page never acknowledges an entry
      Given member "worker" sent direct message "need a decision" to "ops" in space "demo"
      And the operator signed in with the token of admin "ops"
      When the operator opens the inbox page
      Then the entry is still in the inbox of admin "ops"

    Scenario: The inbox page offers no acknowledge button
      Given the operator signed in with the token of admin "ops"
      When the operator requests any of the mutating methods on the inbox route
      Then the response status is 405

    Scenario: The web GUI does not take the admin's inbox stream slot
      Given the operator signed in with the token of admin "ops"
      When the operator opens the inbox page and leaves it open
      Then admin "ops" can still open GET /v1/inbox/stream with its bearer token
      And the web GUI holds no inbox stream
