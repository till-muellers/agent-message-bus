Feature: Listing and cancelling pending invites in the web GUI
  As the operator of a bus
  I want to see the pending invites of a space and cancel one of them
  So that a code I no longer want stops working before it expires

  # PO-3 (round 6): cancelling an invite is NOT a tool. There is no bus_cancel_invite in
  # the registry, so omp and MCP agents cannot cancel invites; bus_revoke_member still
  # cancels every pending code of a name. Cancelling exists only as a web API endpoint
  # of the GUI. Moved here from docs/design/specs/findings/invite-cancel.feature.
  #
  # Invite codes are stored hashed, so the GUI needs a handle that is not the secret:
  # every invite gets an opaque id (the old findings option C). It is added by a
  # numbered migration after 0001_schema (never folded into it; docs/design/specs/REVIEW.md §11)
  # and returned by the invites service's pending listing. bus_invite_member's result is
  # unchanged, and /bus spawn exposes no cancel handle.
  #
  # Routes (session cookie + CSRF, like every GUI mutation in gui-mutations.feature):
  #   GET    /web/api/spaces/{space}/invites        pending invites: [{id, name, expiresAt, invitedBy}]
  #   DELETE /web/api/spaces/{space}/invites/{id}   cancel one pending invite
  # The GUI session must have leader rights in the space: a leader of it or an admin.
  # Cancelling is idempotent and never reveals whether an id existed: an unknown,
  # redeemed, expired or already cancelled id answers {"cancelled": false}.
  # Every cancel writes one server log line (PO-1).

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted admin "ops"
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And the operator signed in with the token of admin "ops"

  Rule: The GUI lists pending invites with an id

    Scenario: A pending invite is listed with its id, name, expiry and inviter
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the operator requests GET /web/api/spaces/demo/invites
      Then the response lists one invite with an id, name "worker", an expiry and invitedBy "lead"
      And the response does not contain the invite code

    Scenario: Redeemed and expired invites are not listed
      Given leader "lead" created an invite code for member "a" in space "demo"
      And that code was accepted
      And leader "lead" created an invite code for member "b" in space "demo"
      And 10 minutes pass
      When the operator requests GET /web/api/spaces/demo/invites
      Then the response lists no invite

  Rule: The GUI cancels one pending invite

    Scenario: The operator cancels a pending invite
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the operator cancels that invite by its id in space "demo" with a CSRF token
      Then the response is {"cancelled": true}
      And posting the code to /v1/invites/accept fails with unauthenticated "invalid or expired invite code"
      And no agent "worker" exists yet

    Scenario: Cancelling one invite keeps another of the same name usable
      Given leader "lead" created two invite codes for member "worker" in space "demo"
      When the operator cancels the first invite by its id
      Then the second code is still accepted

    Scenario: Cancelling a member's new invite does not remove the member
      Given leader "lead" invited member "worker" to space "demo"
      And leader "lead" created another invite code for member "worker" in space "demo"
      When the operator cancels that invite by its id
      Then "worker" is still a member of space "demo"

    Scenario Outline: An id that is not pending reports cancelled false (<case>)
      Given <state>
      When the operator cancels invite id <id> in space "demo"
      Then the response is {"cancelled": false}

      Examples:
        | case              | state                                                    | id               |
        | unknown           | no invite exists                                         | "nope"           |
        | already cancelled | the operator cancelled the invite of "worker"            | of that invite   |
        | redeemed          | the invite of "worker" was accepted                      | of that invite   |

    Scenario: An id of another space is not cancelled there
      Given leader "lead" created space "other"
      And leader "lead" created an invite code for member "worker" in space "other"
      When the operator cancels that invite's id in space "demo"
      Then the response is {"cancelled": false}
      And the code is still accepted

    Scenario: A cancel writes one log line
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the operator cancels that invite by its id
      Then the server logs one info line with admin "ops", space "demo", target "worker" and action "cancel-invite"

  Rule: Access

    Scenario: A cancel without a CSRF token is refused
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the operator cancels that invite without a CSRF token
      Then the response status is 403
      And the code is still accepted

    Scenario: A leader session cancels in its own space
      Given the operator signed in with the token of leader "lead"
      And leader "lead" created an invite code for member "worker" in space "demo"
      When the operator cancels that invite by its id with a CSRF token
      Then the response is {"cancelled": true}

    Scenario: A leader session of another space is refused
      Given the admin minted leader "lead2"
      And the operator signed in with the token of leader "lead2"
      When the operator requests GET /web/api/spaces/demo/invites
      Then the response status is 403

    Scenario: A member session is refused
      Given leader "lead" invited member "worker" to space "demo"
      And the operator signed in with the token of member "worker"
      When the operator requests GET /web/api/spaces/demo/invites
      Then the response status is 403

    Scenario: There is no cancel tool
      When the admin calls GET /v1/tools
      Then the list does not contain "bus_cancel_invite"
