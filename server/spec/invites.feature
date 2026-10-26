Feature: Invite codes
  As a leader of a space
  I want to hand a new worker a short, single-use code instead of a long-lived token
  So that the worker can join my space by itself without anyone copying member tokens around

  # An invite code has 8 characters from "23456789abcdefghijkmnpqrstuvwxyz", shown as xxxx-xxxx.
  # Codes are stored hashed, expire 10 minutes after creation and can be redeemed once.
  # An invite may carry a task. It is stored with the inviter's name and, when the code is
  # accepted, delivered as the member's first inbox entry: a direct_message from the inviter.
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

    Scenario: Deleting the space removes its invite codes
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When leader "lead" deletes space "demo"
      Then no invite code is stored

  Rule: An invalid invite task names the task field (bus_invite_member)

    # The message body and thread title limits keep naming "body" and "title"
    # (direct-messages.feature, topics.feature, blackboards.feature); only the
    # invite task names itself.

    Scenario Outline: The task follows the body limit but names itself (<case>)
      When leader "lead" invites member "worker" to space "demo" with task <task>
      Then the call fails with invalid "task must be 1..65536 bytes"
      And no invite code is stored

      Examples:
        | case     | task           |
        | empty    | ""             |
        | too long | of 65537 bytes |

    Scenario: A task at the limit is accepted
      When leader "lead" invites member "worker" to space "demo" with task of 65536 bytes
      Then the call returns name "worker", space "demo" and an invite code

    Scenario: An invite without a task is accepted
      When leader "lead" invites member "worker" to space "demo"
      Then the call returns name "worker", space "demo" and an invite code

  Rule: An invite code is redeemed once for a member token (POST /v1/invites/accept)

    # Request: POST /v1/invites/accept with "Authorization: Bearer <code>" and no body.
    # This route is not behind the token middleware: the code itself is the bearer.

    Scenario: Accepting a code returns a member token and joins the space
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the code is posted to /v1/invites/accept
      Then the status is 200 and the body is {"name":"worker","space":"demo","token":"amb_…"} without a task
      And the token authenticates as member "worker" bound to space "demo"
      And "worker" is a member of space "demo"

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

  Rule: The invite records who invited (server)

    Scenario: An invite code remembers the inviting leader
      When leader "lead" invites member "worker" to space "demo" with task "fix the bug"
      Then the stored invite records "lead" as the inviter

    Scenario: A second leader's invite records that leader
      Given the admin minted leader "lead2"
      And leader "lead2" joined space "demo"
      When leader "lead2" invites member "worker" to space "demo" with task "write the docs"
      Then the stored invite records "lead2" as the inviter

    Scenario: An invite by an admin records the admin's name
      Given the admin minted admin "ops"
      When admin "ops" invites member "worker" to space "demo" with task "audit the board"
      Then the stored invite records "ops" as the inviter

  Rule: Accepting an invite with a task stores it as a direct message (server)

    Scenario: The task becomes the member's first inbox entry
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then the status is 200 and the body is {"name":"worker","space":"demo","token":"amb_…"}
      And the body has no "task" field
      And member "worker" has exactly one inbox entry in space "demo"
      And that entry has type "direct_message", from "lead", critical false and body "fix the bug"

    Scenario: The task entry is published after the admission commits
      Given a listener on the live channel of "worker"
      And leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then the listener receives exactly one wake-up signal after the transaction commits

    Scenario: An invite without a task delivers nothing
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the code is posted to /v1/invites/accept
      Then member "worker" has no inbox entries

    Scenario: A failed admission delivers nothing and keeps the code
      Given leader "lead" created an invite code for member "x" in space "demo" with task "fix the bug"
      And the admin minted leader "x"
      When the code is posted to /v1/invites/accept
      Then the call fails with conflict "agent x is a leader agent"
      And no inbox entry exists for "x"
      And the invite code is still stored

    Scenario: The task is sent from the inviter even if the inviter has left the space since
      Given the admin minted leader "lead2"
      And leader "lead2" joined space "demo"
      And leader "lead2" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And leader "lead2" left space "demo"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "lead2" with body "fix the bug"

    Scenario: A task from an admin arrives from the admin's name
      Given the admin minted admin "ops"
      And admin "ops" created an invite code for member "worker" in space "demo" with task "audit the board"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "ops" with body "audit the board"

    Scenario: A task from the bootstrap admin arrives from "admin"
      Given the bootstrap admin created an invite code for member "worker" in space "demo" with task "audit the board"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "admin" with body "audit the board"

    Scenario: Re-inviting an existing member with a task sends another direct message
      Given leader "lead" invited member "worker" to space "demo"
      And leader "lead" created an invite code for member "worker" in space "demo" with task "second task"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "lead" with body "second task"

    Scenario: A client without the omp extension reads the task from its inbox
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And the code was accepted and returned a member token
      When member "worker" reads its inbox with that token
      Then the call returns one entry with type "direct_message", from "lead" and body "fix the bug"
