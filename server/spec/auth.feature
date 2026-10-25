Feature: Authentication and leader minting
  As an operator of the agent message bus
  I want every call authenticated by an opaque bearer token tied to an admin, leader or member identity
  So that only agents I minted or that leaders invited can act on the bus

  # Tokens:
  #   - admin:  the single admin token the server is started with; never stored in the database.
  #   - leader: minted by the admin with bus_mint_leader; not bound to any space.
  #   - member: minted by a leader with bus_invite_member; bound to exactly one space.
  # Agent names are global: one name is either a leader agent or a member agent, never both.
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

    Scenario: The admin token authenticates as the admin
      When the admin asks who it is
      Then the call returns kind "admin"

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

    Scenario: Re-inviting an existing member reuses the agent and issues an additional token
      Given the admin minted leader "lead"
      And leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When leader "lead" invites member "worker" to space "demo" again
      Then the second token belongs to the same member agent "worker"
      And the second token differs from the first one
