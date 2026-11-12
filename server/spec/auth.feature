Feature: Authentication and leader minting
  As an operator of the agent message bus
  I want every call authenticated by an opaque bearer token tied to an admin, leader or member identity
  So that only agents I minted or that leaders invited can act on the bus

  # Tokens:
  #   - admin:  the instance admin token the server is started with, which authenticates as the
  #             bootstrap admin "admin"; never stored in the database.
  #   - leader: minted by the admin with bus_mint_leader; not bound to any space.
  #   - member: minted by accepting an invite code from bus_invite_member; bound to exactly one space.
  # Agent names are global: one name is an admin, a leader or a member agent, never two of them.
  # Tokens carry no expiry; a token stops working only when it is removed
  # (member revoked, member left, or the member's space deleted).

  Background:
    Given the bus was started with an admin token

  Rule: Every call presents a bearer token

    Scenario Outline: Calls without a usable bearer credential are rejected (<case>)
      When a caller calls the bus with <credential>
      Then the call fails with unauthenticated "bearer token required"

      Examples:
        | case         | credential                                  |
        | missing      | no Authorization header                     |
        | wrong scheme | the leader token under the "Basic" scheme   |
        | empty bearer | the "Bearer" scheme and an empty token      |

    Scenario: An unknown token is rejected
      When a caller calls the bus with the token "amb_nope"
      Then the call fails with unauthenticated "invalid token"

    Scenario: An empty token is not a valid token
      # Reached only below the transport; over HTTP an empty bearer fails earlier with "bearer token required".
      When the bus authenticates the token ""
      Then the call fails with unauthenticated "invalid token"

    Scenario: The admin token authenticates as the bootstrap admin
      When the admin asks who it is
      Then the call returns kind "admin" and name "admin"

    Scenario: A leader token authenticates as that leader without a space
      Given the admin minted leader "lead"
      When leader "lead" asks who it is
      Then the call returns kind "leader" and name "lead"
      And the leader token is not bound to any space

    Scenario: A member token authenticates as that member bound to its space
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When member "worker" asks who it is
      Then the call returns kind "member", name "worker" and space "demo"

  Rule: Tokens are opaque random strings stored only as hashes

    Scenario: Minted tokens have a fixed format and are unique
      When the admin mints leader "lead"
      And the admin mints leader "lead" again
      Then each returned token is "amb_" followed by 43 base64url characters
      And the two tokens differ

    Scenario: The database stores only the SHA-256 hash of a token
      When the admin mints leader "lead"
      Then the stored token record holds the hex-encoded SHA-256 of the returned token
      And the stored token record does not hold the plaintext token

    Scenario: Tokens never expire
      Given the admin minted leader "lead" a year ago
      When leader "lead" asks who it is
      Then the call returns kind "leader" and name "lead"

  Rule: Only the admin mints leaders (bus_mint_leader)

    Scenario: The admin mints a new leader
      When the admin mints leader "lead"
      Then the call returns name "lead" and a new leader token
      And the returned token authenticates as leader "lead"

    Scenario: Re-minting an existing leader issues an additional working token
      Given the admin minted leader "lead"
      When the admin mints leader "lead" again
      Then the call returns a token different from the first one
      And the first token still authenticates as leader "lead"
      And the additional token authenticates as the same leader "lead"

    Scenario Outline: Non-admin callers cannot mint leaders (<caller>)
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When <caller> mints leader "other"
      Then the call fails with forbidden "admin token required"

      Examples:
        | caller          |
        | leader "lead"   |
        | member "worker" |

    Scenario: The admin check happens before the name is validated
      Given the admin minted leader "lead"
      When leader "lead" mints leader "Bad Name"
      Then the call fails with forbidden "admin token required"

    Scenario Outline: Leader names must be valid (<case>)
      When the admin mints leader "<name>"
      Then the call succeeds

      Examples:
        | case                  | name                                                            |
        | simple                | lead                                                            |
        | digits and separators | 0a_b-c                                                          |
        | max length (63)       | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa |

    Scenario Outline: Invalid leader names are rejected (<case>)
      When the admin mints leader "<name>"
      Then the call fails with invalid "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"

      Examples:
        | case            | name                                                             |
        | too long (64)   | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa |
        | empty           |                                                                  |
        | leading dash    | -a                                                               |
        | uppercase       | Lead                                                             |
        | space           | a b                                                              |
        | mixed           | Bad Name                                                         |

    Scenario: The name argument is required
      When the admin mints a leader without giving a name
      Then the call fails with invalid "name is required"

    Scenario: Unknown arguments are rejected
      When the admin mints leader "lead" with the extra argument "x"
      Then the call fails with invalid "invalid arguments: json: unknown field \"x\""

  Rule: An agent name keeps the kind it was first created with

    Scenario: A member agent's name cannot be minted as a leader
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When the admin mints leader "worker"
      Then the call fails with conflict "agent worker is a member agent"

    Scenario: A leader agent's name cannot be invited as a member
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      When leader "lead" invites member "lead" to space "demo"
      Then the call fails with conflict "agent lead is a leader agent"

    Scenario: Re-inviting an existing member reuses the agent and replaces its token for that space
      # What else a re-invite resets is in membership.feature.
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When leader "lead" invites member "worker" to space "demo" again
      And the member accepts the new invite code
      Then the second token belongs to the same member agent "worker"
      And the second token differs from the first one
      And the first token fails with unauthenticated "invalid token"

  Rule: A call is re-validated inside its write transaction; a revocation that commits first wins

    # The token is checked at the door and again inside the transaction that acts on it:
    # in the space access gate (every space-scoped call) and in bus_create_space,
    # bus_join_space, bus_mint_admin, bus_mint_leader and bus_revoke_admin. A call
    # authenticated just before a revocation (bus_revoke_admin, bus_revoke_member, leaving,
    # deleting the space) or a rotation of the instance admin token, whose transaction runs
    # after it, fails and leaves nothing behind. Only the token that authenticated the call
    # counts: another token of the same agent does not.

    Scenario Outline: A revoked admin's call in flight fails and leaves nothing behind (<call>)
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And the admin minted admins "ops" and "web"
      And admin "ops" was authenticated for <call>
      When the admin revokes admin "ops" before that call's transaction runs
      Then the call fails with unauthenticated "invalid token"
      And no space "orphan" exists
      And space "demo" still has participants "lead" and "worker", and admin "ops" is none of them
      And admin "web" still has its token

      Examples:
        | call                                                          |
        | bus_create_space {"space":"orphan"}                           |
        | bus_join_space {"space":"demo"}                               |
        | bus_invite_member {"space":"demo","name":"late"}              |
        | bus_revoke_member {"space":"demo","name":"worker"}            |
        | bus_revoke_admin {"name":"web"}                               |
        | bus_delete_space {"space":"demo"}                             |

    Scenario Outline: A mint in flight across the caller's revocation issues no token (<caller>, <call>)
      Given the admin minted admin "ops"
      And <caller> was authenticated for <call>
      When <revocation> before that call's transaction runs
      Then the call fails with unauthenticated "invalid token"
      And no token was issued
      And no agent "late" exists

      Examples:
        | caller                                  | call                             | revocation                                  |
        | admin "ops"                             | bus_mint_admin {"name":"ops"}    | the admin revokes admin "ops"               |
        | admin "ops"                             | bus_mint_leader {"name":"late"}  | the admin revokes admin "ops"               |
        | the admin with the instance admin token | bus_mint_admin {"name":"ops"}    | the instance admin token is rotated         |
        | the admin with the instance admin token | bus_mint_leader {"name":"late"}  | the instance admin token is rotated         |

    Scenario: The old token of a re-invited member does not act for the new membership
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And member "worker" was authenticated with its first token for bus_leave_space {"space":"demo"}
      When leader "lead" revokes member "worker" and invites "worker" to space "demo" again before that call's transaction runs
      Then the call fails with unauthenticated "invalid token"
      And "worker" is still a member of space "demo"

    Scenario: A bootstrap admin call in flight across a token rotation fails
      Given the admin was authenticated with the instance admin token for bus_create_space {"space":"orphan"}
      When the instance admin token is rotated before that call's transaction runs
      Then the call fails with unauthenticated "invalid token"
      And no space "orphan" exists
