Feature: Invites in the web GUI
  As a human operator of a bus
  I want to see which member invitations are still open in any space
  So that I know who has not joined yet

  # Invite codes are stored hashed, so a pending code cannot be shown again
  # after it was created. The web GUI therefore shows the invited name, the
  # expiry and an opaque invite id (PO-3; numbered migration after 0001_schema),
  # never the code. Cancelling one invite by its id is gui-invite-cancel.feature.
  #
  # The operator signs in with an admin token. An admin acts as a leader in
  # every space without joining it (see docs/design/specs/admin/admin-role.feature):
  # it sees the open invites of every space, not only of the spaces it
  # participates in.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" created space "other"

  Rule: Listing the open invites of a space

    Scenario: An open invite is listed with its invited name and expiry
      Given leader "lead" created an invite code for member "worker" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists the open invites:
        | name   | space |
        | worker | demo  |
      And the page shows the expiry of the invite
      And the page does not show the invite code

    Scenario: A redeemed invite is not listed any more
      Given leader "lead" created an invite code for member "worker" in space "demo"
      And the code was accepted
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists no open invite
      And the page lists member "worker" as a participant

    Scenario: An expired invite is not listed
      Given leader "lead" created an invite code for member "worker" in space "demo"
      And 10 minutes pass
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists no open invite

    Scenario: A revoked member has no open invite
      Given leader "lead" created an invite code for member "worker" in space "demo"
      And leader "lead" revoked member "worker" from space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists no open invite

  Rule: The operator sees the open invites of every space

    Scenario: The open invites of another space are listed
      Given leader "lead" created an invite code for member "worker" in space "other"
      Given the operator signed in with the instance admin token
      When the operator opens the space "other"
      Then the page lists the open invites:
        | name   | space |
        | worker | other |

    Scenario: The space list shows how many invitations are open per space
      Given leader "lead" created an invite code for member "worker" in space "demo"
      And leader "lead" created an invite code for member "other" in space "other"
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the space "demo" shows 1 open invite
      And the space "other" shows 1 open invite

    Scenario: A leader that does not participate in the space sees no invite
      Given the admin minted leader "stranger"
      And leader "lead" created an invite code for member "worker" in space "demo"
      Given the operator signed in with the token of leader "stranger"
      When the operator opens the space "demo"
      Then the response status is 403
