Feature: Admin management in the web GUI
  As a human operator of a bus
  I want to list, mint and revoke admins from the web GUI
  So that every person gets their own admin identity and I can withdraw it again

  # Moved from docs/design/specs/web/gui-admins.feature (slice W2).
  #
  # The administration area of the web GUI. It is backed by the three admin
  # tools (see ../admin-tokens.feature), called in-process with the session's
  # principal, so access rules and error messages match /v1/tools:
  #   bus_list_admins   the admins of the instance
  #   bus_mint_admin    a new admin with its own token
  #   bus_revoke_admin  withdrawing a minted admin
  #
  # The web GUI **may mint admins** (PO ruling, round 5): minting from the
  # browser is part of the administration website, and the minted token is
  # shown once and then never again.
  #
  # Recommendation: **one minted admin per human operator** (and one per
  # GUI installation). The web GUI session is then attributable ("admin ops" did it),
  # revocable on its own (bus_revoke_admin), and the bootstrap admin's
  # instance token stays on the host for the CLI (stop, rotate-token).
  #
  # Routes (admin sessions only, else 403 forbidden "admin token required"):
  #   GET  /web/api/admins                 the admin list (bus_list_admins)
  #   POST /web/api/admins {"name"}        mint an admin (bus_mint_admin)
  #   POST /web/api/admins/{name}/revoke   revoke a minted admin (bus_revoke_admin)
  #
  # Both POST routes need the header X-CSRF-Token equal to the session's csrf
  # (from sign-in or GET /web/session); a missing or foreign token is 403
  # forbidden "invalid CSRF token" with no side effect. Every response carries
  # Cache-Control: no-store.
  #
  # The minted token is shown once, like an invite code: it is stored hashed
  # and can never be shown again.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists

  Rule: The operator sees the admins of the instance

    Scenario: The admin list names every admin
      Given the admin minted admin "ops"
      And the admin minted admin "web"
      When the operator opens the admin list
      Then the page lists:
        | name  | bootstrap | tokens | spaces |
        | admin | true      | 0      |        |
        | ops   | false     | 1      |        |
        | web   | false     | 1      |        |

    Scenario: The admin list shows the spaces an admin joined
      Given the admin minted admin "ops"
      And the admin minted leader "lead"
      And leader "lead" created space "demo"
      And admin "ops" joined space "demo"
      When the operator opens the admin list
      Then the page shows admin "ops" in space "demo"

    Scenario: The admin list shows how many tokens an admin holds
      Given the admin minted admin "ops"
      And the admin minted admin "ops" again
      When the operator opens the admin list
      Then the page shows admin "ops" with 2 tokens

    Scenario: A revoked admin stays in the list without tokens
      Given the admin minted admin "ops"
      And the admin revoked admin "ops"
      When the operator opens the admin list
      Then the page lists admin "ops" with 0 tokens

  Rule: The operator mints an admin

    Scenario: The operator mints an admin
      When the operator mints admin "ops"
      Then the response status is 200
      And the page shows the token of admin "ops" once
      And the token authenticates as admin "ops"

    Scenario: The token is shown once and never again
      When the operator mints admin "ops"
      And the operator reloads the admin list
      Then the page does not show the token of admin "ops"
      And the page lists admin "ops" with 1 token

    Scenario: The operator mints an admin of an instance it does not lead
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      When the operator mints admin "web"
      Then the response status is 200
      And the page shows the token of admin "web" once

    Scenario: An invalid admin name is refused
      When the operator mints admin "Bad Name"
      Then the response status is 400
      And the page shows the error "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"
      And no admin was minted

    Scenario: The bootstrap name cannot be minted
      When the operator mints admin "admin"
      Then the response status is 409
      And the page shows the error "admin is the bootstrap admin; its token is the instance admin token"
      And no admin was minted

    Scenario: A name of another kind cannot be minted as an admin
      Given the admin minted leader "lead"
      When the operator mints admin "lead"
      Then the response status is 409
      And the page shows the error "agent lead is a leader agent"

    Scenario: Minting needs a session
      Given the operator signed out
      When the operator mints admin "ops"
      Then the response status is 401
      And no admin was minted

  Rule: The operator revokes an admin

    Scenario: The operator revokes a minted admin
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      When the operator revokes admin "ops"
      Then the response status is 200
      And the token of admin "ops" fails with unauthenticated "invalid token"

    Scenario: A revoke ends the revoked admin's web GUI sessions
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      And the operator revokes admin "ops"
      When the operator requests GET /web/api/state
      Then the response status is 401

    Scenario: The bootstrap admin cannot be revoked
      When the operator revokes admin "admin"
      Then the response status is 403
      And the page shows the error "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"

    Scenario: Revoking an admin that does not exist reports it
      When the operator revokes admin "ghost"
      Then the response status is 200
      And the page shows that nothing was removed

    Scenario: Revoking needs a session
      Given the operator signed out
      When the operator revokes admin "ops"
      Then the response status is 401
      And admin "ops" still authenticates

  Rule: Only an admin session reaches the admin area

    Scenario: A leader session sees no admin list
      Given the admin minted leader "lead"
      And the operator signed in with the token of leader "lead"
      When the operator requests GET /web/api/admins
      Then the response status is 403

    Scenario: A leader session cannot mint an admin
      Given the admin minted leader "lead"
      And the operator signed in with the token of leader "lead"
      When the operator mints admin "ops"
      Then the response status is 403
      And no admin was minted

    Scenario: A member session sees no admin list
      Given leader "lead" invited member "worker" to space "demo"
      And the operator signed in with the token of member "worker"
      When the operator requests GET /web/api/admins
      Then the response status is 403
