Feature: Web GUI access and sessions
  As a human operator of a bus
  I want to sign in to a web GUI with my admin token
  So that I can administer my instance without putting a token into a URL

  # The web GUI is a read-only view served by the bus binary itself (single
  # static binary, no extra process). It is a transport, like internal/rpc and
  # internal/admin: it reads through the feature services and owns no SQL.
  #
  # The operator signs in with an **admin token**. Admins are named agents of
  # kind "admin" (see docs/design/specs/admin/admin-tokens.feature): the web GUI
  # identifies the operator by that agent's name — "admin admin" for the
  # bootstrap admin, "admin ops" for a minted one — never by a fixed
  # placeholder.
  #
  # An admin acts as a leader in every space without joining it, so the
  # web GUI shows every space, not only the spaces of one agent.
  #
  # Listen address (PO ruling): the web GUI has its OWN listener, configured by
  # AMB_WEB_ADDR (host:port, any host, any port), separate from the API's AMB_ADDR.
  # The operator decides exposure by choosing the address. Default: 127.0.0.1:8081
  # (loopback, the API's default port + 1). Empty or "off" disables the GUI. The API
  # listener never serves /web/*. Docker images must set AMB_WEB_ADDR explicitly
  # (server/DEPLOYMENT.md documents it); the GUI is then reachable as configured.
  #
  # Routes (on the AMB_WEB_ADDR listener only):
  #   POST /web/session          exchange an admin token for a session cookie
  #   DELETE /web/session        drop the session
  #   GET  /web/api/state        the aggregated state of a view (JSON)
  #   GET  /web/*                the embedded GUI application
  # The GUI is a client application, so it reads JSON and renders in the browser
  # (analysis.md 3.3). The session cookie is HttpOnly, SameSite=Strict and Secure
  # whenever the request arrived over TLS. It holds an opaque session id, never
  # the token.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" created space "other"

  Rule: The web GUI has its own configurable listen address

    Scenario: By default the GUI listens on loopback port 8081
      Given AMB_WEB_ADDR is not set
      When the server starts
      Then the web GUI listens on "127.0.0.1:8081"
      And GET http://127.0.0.1:8081/web/ answers 200

    Scenario Outline: The operator chooses the GUI address (<addr>)
      Given AMB_WEB_ADDR is "<addr>"
      When the server starts
      Then the web GUI listens on "<addr>"
      And GET /web/ on that address answers 200

      Examples:
        | addr           |
        | 127.0.0.1:9000 |
        | 0.0.0.0:8081   |

    Scenario Outline: The GUI can be disabled (<value>)
      Given AMB_WEB_ADDR is "<value>"
      When the server starts
      Then no web GUI listener is opened
      And the API on AMB_ADDR works as usual

      Examples:
        | value |
        |       |
        | off   |

    Scenario: The API listener does not serve the GUI
      Given AMB_ADDR is "127.0.0.1:8080" and AMB_WEB_ADDR is "127.0.0.1:8081"
      When a client requests GET http://127.0.0.1:8080/web/
      Then the response status is 404

    Scenario: A GUI address that cannot be bound stops the start
      Given AMB_WEB_ADDR is the same address as AMB_ADDR
      When the server starts
      Then it exits with an error naming AMB_WEB_ADDR

  Rule: Signing in exchanges an admin token for a session

    Scenario: The instance admin token starts a session as the bootstrap admin
      When the operator signs in with the instance admin token
      Then the response status is 200
      And the response sets a session cookie
      And the session cookie is HttpOnly and SameSite=Strict
      And the response body does not contain the token
      And the session identifies the operator as admin "admin"

    Scenario: A minted admin token starts a session under that admin's name
      Given the admin minted admin "ops"
      When the operator signs in with the token of admin "ops"
      Then the response status is 200
      And the response sets a session cookie
      And the session identifies the operator as admin "ops"

    Scenario: Two operators are two admins with two sessions
      Given the admin minted admin "ops"
      When the operator signs in with the instance admin token
      And the operator signs in with the token of admin "ops"
      Then the first session identifies the operator as admin "admin"
      And the second session identifies the operator as admin "ops"

    Scenario: A leader token starts a session with less reach
      When the operator signs in with the token of leader "lead"
      Then the response status is 200
      And the response sets a session cookie
      And the session shows only the spaces of leader "lead"

    Scenario Outline: An unusable token is rejected (<case>)
      When the operator signs in with <token>
      Then the response status is 401
      And the response body does not contain the token
      And no session cookie is set

      Examples:
        | case        | token                                   |
        | unknown     | amb_nope                                |
        | empty       | ""                                      |
        | wrong kind  | Basic admin-token-0123456789-0123456789 |

    Scenario: A member token may sign in
      Given member "worker" accepted an invite for space "demo"
      When the operator signs in with the token of member "worker"
      Then the response status is 200

  Rule: A session is re-validated on every request

    Scenario: An active session sees the web GUI
      Given the operator signed in with the instance admin token
      When the operator requests GET /web/api/state
      Then the response status is 200

    Scenario: Rotating the instance token ends the bootstrap admin's sessions only
      Given the operator signed in with the instance admin token
      And the admin minted admin "ops"
      And the second operator signed in with the token of admin "ops"
      And the admin token was rotated
      When the operator requests GET /web/api/state
      Then the response status is 401
      When the second operator requests GET /web/api/state
      Then the response status is 200

    Scenario: Revoking an admin ends that admin's sessions
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      And the admin revoked admin "ops"
      When the operator requests GET /web/api/state
      Then the response status is 401

    Scenario: A session of a revoked member stops working
      Given member "worker" accepted an invite for space "demo"
      And the operator signed in with the token of member "worker"
      And leader "lead" revoked member "worker" from space "demo"
      When the operator requests GET /web/api/state
      Then the response status is 401

    Scenario: Signing out drops the session
      Given the operator signed in with the instance admin token
      When the operator signs out
      Then the response status is 200
      And the response clears the session cookie
      When the operator requests GET /web/api/state
      Then the response status is 401

  Rule: The web GUI is read-only in the first milestone

    # In the first milestone no web GUI route changes bus state. The second
    # milestone adds the mutations of gui-mutations.feature (create invite, revoke a
    # member or leader, post to a thread, send to a topic, delete a space behind a
    # confirmation naming the space (PO-4)) and the admin management of
    # gui-admins.feature. Deleting a board or a thread stays in the CLI and the agent tools.

    Scenario: The web GUI exposes no mutating route yet
      Given the operator signed in with the instance admin token
      When the operator requests any of the mutating methods on a web GUI route
      Then the response status is 405

    Scenario: The web GUI never opens an inbox stream
      # The inbox stream authenticates with a bearer token, not with the
      # web GUI session, so the web GUI can never take the stream slot of an
      # admin or an agent.
      Given the operator signed in with the token of leader "lead"
      When the operator requests GET /v1/inbox/stream with the session cookie
      Then the response status is 401
      And leader "lead" can still open its own inbox stream with its bearer token

    Scenario: The web GUI of an admin never takes the admin's own stream slot
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      When the operator opens the web GUI and leaves it open
      Then admin "ops" can still open GET /v1/inbox/stream with its bearer token
