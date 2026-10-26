Feature: Live updates in the web GUI
  As a human operator of a bus
  I want the web GUI to refresh itself while it is open
  So that I see new participants, topics and posts without reloading

  # The bus has one live channel today: GET /v1/inbox/stream, a
  # server-sent-event stream of the calling agent's inbox. It is per agent and
  # the server allows only one stream per agent. Admins are agents too, so
  # they have a stream slot as well — and the operator's own omp session may
  # already hold it. The web GUI must not take that slot, and the inbox
  # carries messages, not space structure.
  #
  # The GUI is a client application (analysis.md 3.3), so it reads JSON. It
  # polls one aggregated state endpoint per view (GET /web/api/state), which
  # needs no new server channel and cannot steal a stream slot. The endpoint composes
  # internal/overview (the read behind bus_space_overview,
  # docs/design/specs/guidance/space-overview.feature) with the pending invites of the space.
  #
  # A change-hint event stream (GET /web/api/events) is the later upgrade; it
  # would need a second broker topic family, because the existing one is
  # per-agent inbox delivery.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And leader "lead" created space "demo"

  Rule: The web GUI refreshes by polling the state endpoint

    Scenario: The state endpoint returns the whole view of a space in one call
      Given leader "lead" created topic "notes" in space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And member "worker" created blackboard "b" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator requests GET /web/api/state for space "demo"
      Then the response status is 200
      And the response body has the participants, the topics, the blackboards and the threads of space "demo"

    Scenario: The state endpoint carries no message bodies
      Given leader "lead" created topic "notes" in space "demo"
      And member "worker" posted to topic "notes" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator requests GET /web/api/state for space "demo"
      Then the response body does not contain the message body of the topic message

    Scenario: The state endpoint carries the open invites of the space
      Given leader "lead" created an invite code for member "new" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator requests GET /web/api/state for space "demo"
      Then the response body lists "new" as an open invite
      And the response body does not contain the invite code

    Scenario: The state of an unknown space is not found
      Given the operator signed in with the instance admin token
      When the operator requests GET /web/api/state for space "ghost"
      Then the response status is 404

    Scenario: The state endpoint without a session is rejected
      When a client requests GET /web/api/state for space "demo"
      Then the response status is 401

  Rule: The web GUI does not hold an inbox stream

    Scenario: The web GUI leaves the inbox stream slot of the leader free
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo" and leaves it open
      Then leader "lead" can open GET /v1/inbox/stream
      And the web GUI holds no inbox stream

    Scenario: The web GUI leaves the inbox stream slot of its own admin free
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      When the operator opens the space "demo" and leaves it open
      Then admin "ops" can open GET /v1/inbox/stream
      And the web GUI holds no inbox stream

    Scenario: A second agent stream still conflicts with the first
      Given leader "lead" opened GET /v1/inbox/stream
      When leader "lead" opens GET /v1/inbox/stream again
      Then the response status is 409

    Scenario: A second admin stream conflicts with the first
      Given the admin minted admin "ops"
      And admin "ops" opened GET /v1/inbox/stream
      When admin "ops" opens GET /v1/inbox/stream again
      Then the response status is 409
