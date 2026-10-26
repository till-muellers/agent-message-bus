Feature: Admin agents, the bootstrap admin and admin tokens
  As the operator of a bus
  I want a bootstrap admin from server startup and further named admins minted by admins
  So that every person or console gets its own admin identity and I can withdraw it again

  # PROPOSAL (docs/design/specs/admin/analysis.md). PO ruling 3.
  #
  # Admins are named agents of kind "admin": one agent row per admin, each with its
  # own inbox and stream slot. Agent names stay global, so a name is a leader, a
  # member or an admin, never two of them.
  #
  # The bootstrap admin:
  #   - is the agent "admin", created on the first start of a database (an existing
  #     row of kind admin is reused);
  #   - authenticates with the instance admin token: AMB_ADMIN_TOKEN if set, else
  #     <datadir>/admin-token. As today, that token is only held in memory as a hash
  #     and never stored in the database;
  #   - is the only admin whose token the CLI uses (stop, rotate-token) and the only
  #     one that may replace the instance admin token (POST /v1/admin/admin-token);
  #   - cannot be revoked. So the bus always has at least one admin, and no
  #     "last admin" rule is needed.
  #
  # Minted admins (bus_mint_admin):
  #   - any admin may mint an admin; re-minting issues an additional token, like
  #     bus_mint_leader;
  #   - tokens are "amb_admin_" + 43 base64url characters (PO ruling Q9), stored as
  #     SHA-256 hashes, not bound to a space, no expiry. The prefix is a label for
  #     humans and secret scanners only and grants nothing by itself: authority comes
  #     from the agents row the token's hash resolves to;
  #   - any admin may revoke a minted admin (bus_revoke_admin), itself included;
  #   - revoking deletes every token of that admin and removes it from every space it
  #     joined (departure path), but keeps the agent row: posts, threads, topics and
  #     boards it authored keep their author. Minting the same name again gives
  #     that agent a new token.
  #
  # Rotating the instance admin token (rotate-token) changes only the bootstrap
  # admin's credential. Minted admin tokens are unaffected. After an incident:
  # rotate, then bus_list_admins and bus_revoke_admin.

  Background:
    Given the bus was started with the admin token "admin-token-0123456789-0123456789-abcdef"

  Rule: The instance admin token is the bootstrap admin "admin"

    Scenario: The instance admin token authenticates as the bootstrap admin
      When the bootstrap admin asks who it is
      Then the call returns kind "admin", name "admin" and spaces []

    Scenario: The bootstrap admin exists once, created on the first start
      Then the bus holds exactly one agent named "admin" with kind "admin"

    Scenario: A restart reuses the bootstrap admin
      Given the bootstrap admin joined space "demo" created by leader "lead"
      When the server restarts with the same database
      Then the bus still holds exactly one agent named "admin"
      And the bootstrap admin is still a participant of space "demo"

    Scenario: The instance admin token is not stored in the database
      Then no token record holds the SHA-256 of "admin-token-0123456789-0123456789-abcdef"

    Scenario: "admin" cannot be minted as a leader
      When the bootstrap admin mints leader "admin"
      Then the call fails with conflict "agent admin is an admin agent"

  Rule: Admins mint admins (bus_mint_admin)

    Scenario: The bootstrap admin mints an admin
      When the bootstrap admin calls bus_mint_admin with {"name":"ops"}
      Then the call returns name "ops" and a token
      And the token is "amb_admin_" followed by 43 base64url characters
      And the token authenticates as admin "ops"
      And admin "ops" is not a participant of any space

    Scenario: A minted admin token resolves through the tokens table
      # Authenticate compares the instance token in memory first; a minted token never
      # matches it and is then looked up by hash in tokens JOIN agents.
      Given the bootstrap admin minted admin "ops"
      Then the token record of admin "ops" holds the SHA-256 of the returned token
      And the token authenticates as admin "ops", not as the bootstrap admin

    Scenario: The prefix carries no authority
      When a caller calls the bus with the token "amb_admin_" followed by 43 random base64url characters
      Then the call fails with unauthenticated "invalid token"

    Scenario: Leader and member tokens keep the plain prefix
      When the bootstrap admin mints leader "lead"
      Then the token is "amb_" followed by 43 base64url characters and does not start with "amb_admin_"

    Scenario: A minted admin mints another admin
      Given the bootstrap admin minted admin "ops"
      When admin "ops" calls bus_mint_admin with {"name":"web"}
      Then the call returns name "web" and a token

    Scenario: Re-minting an admin issues an additional working token
      Given the bootstrap admin minted admin "ops"
      When the bootstrap admin mints admin "ops" again
      Then the call returns a token different from the first one
      And both tokens authenticate as the same admin "ops"

    Scenario: A minted admin mints leaders
      Given the bootstrap admin minted admin "ops"
      When admin "ops" calls bus_mint_leader with {"name":"lead"}
      Then the call returns name "lead" and a new leader token

    Scenario Outline: Only admins mint admins (<caller>)
      Given the bootstrap admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When <caller> calls bus_mint_admin with {"name":"x"}
      Then the call fails with forbidden "admin token required"

      Examples:
        | caller          |
        | leader "lead"   |
        | member "worker" |

    Scenario: The bootstrap name cannot be minted
      # A database token for "admin" would survive rotate-token.
      When the bootstrap admin calls bus_mint_admin with {"name":"admin"}
      Then the call fails with conflict "admin is the bootstrap admin; its token is the instance admin token"

    Scenario Outline: An admin name keeps its kind (<kind>)
      Given the bootstrap admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When the bootstrap admin calls bus_mint_admin with {"name":"<name>"}
      Then the call fails with conflict "agent <name> is a <kind> agent"

      Examples:
        | name   | kind   |
        | lead   | leader |
        | worker | member |

    Scenario: Admin names follow the agent name rule
      When the bootstrap admin calls bus_mint_admin with {"name":"Bad Name"}
      Then the call fails with invalid "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"

    Scenario: The admin check happens before the name is validated
      Given the bootstrap admin minted leader "lead"
      When leader "lead" calls bus_mint_admin with {"name":"Bad Name"}
      Then the call fails with forbidden "admin token required"

  Rule: Admins see the admins (bus_list_admins)

    Scenario: The list holds every admin agent, by name
      Given the bootstrap admin minted admin "ops"
      And the bootstrap admin minted admin "ops" again
      And the bootstrap admin minted admin "web"
      And admin "web" was revoked
      And the bootstrap admin minted leader "lead"
      And leader "lead" created space "demo"
      And admin "ops" joined space "demo"
      When admin "ops" calls bus_list_admins
      Then the call returns:
        | name  | bootstrap | tokens | spaces |
        | admin | true      | 0      |        |
        | ops   | false     | 2      | demo   |
        | web   | false     | 0      |        |

    Scenario: Leaders and members cannot list admins
      Given the bootstrap admin minted leader "lead"
      When leader "lead" calls bus_list_admins
      Then the call fails with forbidden "admin token required"

  Rule: Admins revoke minted admins (bus_revoke_admin)

    Scenario: An admin revokes another admin
      Given the bootstrap admin minted admin "ops"
      And the bootstrap admin minted admin "web"
      When admin "ops" calls bus_revoke_admin with {"name":"web"}
      Then the call returns {"deleted": true}
      And every token of admin "web" fails with unauthenticated "invalid token"

    Scenario: Revoking an admin removes it from every space it joined
      Given the bootstrap admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" created space "other"
      And the bootstrap admin minted admin "web"
      And admin "web" joined space "demo"
      And admin "web" joined space "other"
      And admin "web" has inbox entries in spaces "demo" and "other"
      When the bootstrap admin calls bus_revoke_admin with {"name":"web"}
      Then admin "web" is not a participant of space "demo" or "other"
      And admin "web" has no inbox entry
      And admin "web" has no topic or thread subscription

    Scenario: A revoked admin's authored content keeps its author
      Given the bootstrap admin minted leader "lead"
      And leader "lead" created space "demo"
      And the bootstrap admin minted admin "web"
      And admin "web" created blackboard "audit" in space "demo"
      When the bootstrap admin calls bus_revoke_admin with {"name":"web"}
      Then the blackboard "audit" of space "demo" still has owner "web"

    Scenario: Minting a revoked admin again gives the same agent a new token
      Given the bootstrap admin minted admin "web"
      And admin "web" was revoked
      When the bootstrap admin calls bus_mint_admin with {"name":"web"}
      Then the new token authenticates as admin "web" with the same agent id as before

    Scenario: An admin may revoke itself
      Given the bootstrap admin minted admin "ops"
      When admin "ops" calls bus_revoke_admin with {"name":"ops"}
      Then the call returns {"deleted": true}
      And the token of admin "ops" fails with unauthenticated "invalid token"

    Scenario Outline: The bootstrap admin cannot be revoked (<caller>)
      Given the bootstrap admin minted admin "ops"
      When <caller> calls bus_revoke_admin with {"name":"admin"}
      Then the call fails with forbidden "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"

      Examples:
        | caller              |
        | the bootstrap admin |
        | admin "ops"         |

    Scenario: Revoking an unknown name removes nothing
      When the bootstrap admin calls bus_revoke_admin with {"name":"ghost"}
      Then the call returns {"deleted": false}

    Scenario: Revoking an already revoked admin removes nothing
      Given the bootstrap admin minted admin "web"
      And admin "web" was revoked
      When the bootstrap admin calls bus_revoke_admin with {"name":"web"}
      Then the call returns {"deleted": false}

    Scenario: bus_revoke_admin does not revoke leaders
      Given the bootstrap admin minted leader "lead"
      When the bootstrap admin calls bus_revoke_admin with {"name":"lead"}
      Then the call fails with conflict "agent lead is a leader agent"

    Scenario: Only admins revoke admins
      Given the bootstrap admin minted admin "ops"
      And the bootstrap admin minted leader "lead"
      When leader "lead" calls bus_revoke_admin with {"name":"ops"}
      Then the call fails with forbidden "admin token required"

  Rule: Every admin mint and revoke is logged

    # One structured info line (slog, JSON handler on stderr, so it lands in the
    # log file under `start`). Never contains a token.

    Scenario: Minting an admin logs one line
      Given the bootstrap admin minted admin "ops"
      When admin "ops" calls bus_mint_admin with {"name":"web"}
      Then the server logs one info line with msg "admin minted", by "ops", name "web"
      And no log line contains the returned token

    Scenario: Re-minting logs again
      Given the bootstrap admin minted admin "ops"
      When the bootstrap admin mints admin "ops" again
      Then the server logs one info line with msg "admin minted", by "admin", name "ops"

    Scenario: Revoking an admin logs one line with the outcome
      Given the bootstrap admin minted admin "web"
      When the bootstrap admin calls bus_revoke_admin with {"name":"web"}
      Then the server logs one info line with msg "admin revoked", by "admin", name "web", deleted true

    Scenario: A no-op revoke is logged too
      When the bootstrap admin calls bus_revoke_admin with {"name":"ghost"}
      Then the server logs one info line with msg "admin revoked", by "admin", name "ghost", deleted false

    Scenario: Refused calls log no audit line
      Given the bootstrap admin minted leader "lead"
      When leader "lead" calls bus_mint_admin with {"name":"x"}
      Then the server logs no "admin minted" line

  Rule: Rotating the instance admin token changes only the bootstrap admin's credential

    Scenario: rotate-token keeps the bootstrap admin and its participations
      Given an instance is running with the token from <datadir>/admin-token
      And the bootstrap admin joined space "demo" created by leader "lead"
      When I run "agent-message-bus rotate-token"
      Then a call with the old token gets 401 unauthenticated
      And the new token authenticates as the bootstrap admin "admin"
      And the bootstrap admin is still a participant of space "demo"

    Scenario: rotate-token does not revoke minted admins
      Given an instance is running with the token from <datadir>/admin-token
      And the bootstrap admin minted admin "ops"
      When I run "agent-message-bus rotate-token"
      Then the token of admin "ops" still authenticates

    Scenario: A minted admin cannot replace the instance admin token
      Given the bootstrap admin minted admin "ops"
      When a client from 127.0.0.1 calls POST /v1/admin/admin-token with the token of admin "ops"
      Then the response status is 403
      And the call fails with forbidden "only the bootstrap admin may replace the instance admin token"

  Rule: The admin endpoints stay loopback-only

    # Instance lifecycle endpoints. The loopback rule is unchanged. The bus_* admin
    # tools (bus_mint_leader, bus_mint_admin, bus_list_admins, bus_revoke_admin) are
    # not loopback-only, like every tool (analysis.md §6).

    Scenario Outline: Guard on <route> from <remote> with the <token> token
      Given the bootstrap admin minted admin "ops"
      And the bootstrap admin minted leader "lead"
      When a client from <remote> calls POST <route> with the <token> token
      Then the response status is 403
      And the call fails with forbidden "<message>"

      Examples:
        | route                 | remote    | token     | message                                                       |
        | /v1/admin/shutdown    | 10.0.0.2  | bootstrap | admin endpoints are only available from localhost             |
        | /v1/admin/shutdown    | 10.0.0.2  | ops       | admin endpoints are only available from localhost             |
        | /v1/admin/shutdown    | 127.0.0.1 | lead      | admin token required                                          |
        | /v1/admin/admin-token | 127.0.0.1 | ops       | only the bootstrap admin may replace the instance admin token |

    Scenario: Any admin may shut the instance down from loopback
      Given the bootstrap admin minted admin "ops"
      When a client from 127.0.0.1 calls POST /v1/admin/shutdown with the token of admin "ops"
      Then the response status is 200
      And the server shuts down gracefully

    Scenario: bus_mint_admin works from a remote address
      When a client from 10.0.0.2 calls POST /v1/tools/bus_mint_admin with the instance admin token and {"name":"ops"}
      Then the response status is 200
