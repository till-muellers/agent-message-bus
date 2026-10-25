Feature: Invite codes
  As a leader of a space
  I want to hand a new worker a short, single-use code instead of a long-lived token
  So that the worker can join my space by itself without anyone copying member tokens around

  # An invite code has 8 characters from "23456789abcdefghijkmnpqrstuvwxyz", shown as xxxx-xxxx.
  # Codes are stored hashed, expire 10 minutes after creation and can be redeemed once.
  # An invite may carry a task: the member's first prompt, returned once on accept.
  # bus_revoke_member cancels every pending code for the revoked name in the space.
  # Redeeming is case-insensitive and ignores dashes and other separators.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"

  Rule: Leaders create invite codes (bus_invite_member)

    Scenario: A leader creates an invite code
      When leader "lead" invites member "worker" to space "demo"
      Then the call returns name "worker", space "demo", a code matching "^[2-9a-km-np-z]{4}-[2-9a-km-np-z]{4}$" and expires_at 10 minutes from now
      And only the hash of the code is stored
      And no agent "worker" exists yet

    Scenario: The invitee name is checked when the code is created
      Given the admin minted leader "boss"
      When leader "lead" invites member "boss" to space "demo"
      Then the call fails with conflict "agent boss is a leader agent"
      And no invite code is stored

    Scenario: A leader attaches a first prompt to an invite code
      When leader "lead" invites member "worker" to space "demo" with task "fix the bug"
      Then the call returns name "worker", space "demo" and an invite code

    Scenario Outline: The task follows the message body rule (<case>)
      When leader "lead" invites member "worker" to space "demo" with task <task>
      Then the call fails with invalid "body must be 1..65536 bytes"
      And no invite code is stored

      Examples:
        | case     | task           |
        | empty    | ""             |
        | too long | of 65537 bytes |

    Scenario: Deleting the space removes its invite codes
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When leader "lead" deletes space "demo"
      Then no invite code is stored

  Rule: An invite code is redeemed once for a member token (POST /v1/invites/accept)

    # Request: POST /v1/invites/accept with "Authorization: Bearer <code>" and no body.
    # This route is not behind the token middleware: the code itself is the bearer.

    Scenario: Accepting a code returns a member token and joins the space
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the code is posted to /v1/invites/accept
      Then the status is 200 and the body is {"name":"worker","space":"demo","token":"amb_…"} without a task
      And the token authenticates as member "worker" bound to space "demo"
      And "worker" is a member of space "demo"

    Scenario: Accepting a code returns its task
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then the status is 200 and the body is {"name":"worker","space":"demo","token":"amb_…","task":"fix the bug"}

    Scenario: A code works only once
      Given leader "lead" created an invite code for member "worker" in space "demo"
      And the code was accepted
      When the code is posted to /v1/invites/accept again
      Then the call fails with unauthenticated "invalid or expired invite code"

    Scenario: A code expires after 10 minutes
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When 10 minutes pass
      And the code is posted to /v1/invites/accept
      Then the call fails with unauthenticated "invalid or expired invite code"

    Scenario: Upper case and a missing dash are accepted
      Given leader "lead" created invite code "k7m2-x9pq" for member "worker" in space "demo"
      When "K7M2X9PQ" is posted to /v1/invites/accept
      Then the status is 200

    Scenario Outline: Unknown or malformed codes are rejected (<case>)
      When "<code>" is posted to /v1/invites/accept
      Then the call fails with unauthenticated "invalid or expired invite code"

      Examples:
        | case      | code      |
        | unknown   | 2345-6789 |
        | too short | 2345      |

    Scenario: A missing bearer is rejected
      When /v1/invites/accept is posted without an Authorization header
      Then the call fails with unauthenticated "bearer token required"

    Scenario: A failed admission does not consume the code
      Given leader "lead" created an invite code for member "x" in space "demo"
      And the admin minted leader "x"
      When the code is posted to /v1/invites/accept
      Then the call fails with conflict "agent x is a leader agent"
      And the invite code is still stored

  Rule: Revoking a name cancels its pending invite codes (bus_revoke_member)

    Scenario: Revoking a name that only has a pending code cancels the code
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}
      When the code is posted to /v1/invites/accept
      Then the call fails with unauthenticated "invalid or expired invite code"

    Scenario: Revoking a member also cancels its pending codes
      Given leader "lead" invited member "worker" to space "demo"
      And leader "lead" created an invite code for member "worker" in space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}
      When the code is posted to /v1/invites/accept
      Then the call fails with unauthenticated "invalid or expired invite code"

    Scenario: Codes for other names stay valid
      Given leader "lead" created an invite code for member "other" in space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then the call returns {"deleted": false}
      When the code is posted to /v1/invites/accept
      Then the status is 200
