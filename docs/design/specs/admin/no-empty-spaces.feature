Feature: A space never exists without participants
  As the operator of a bus
  I want a space to disappear when its last participant departs
  So that no orphaned space with topics, boards and invites lingers

  # PROPOSAL (docs/design/specs/admin/analysis.md D15). PO ruling Q6: general invariant, any role.
  #
  # Every departure path ends in spaces.Service.removeParticipant: leave, bus_revoke_member
  # (member or, by an admin, leader), and bus_revoke_admin (once per joined space).
  # After it deletes the participant row, in the same transaction, it counts the
  # remaining participants. At zero it deletes the space through the existing
  # delete path (deleteSpace, spaces/repository.go:128). The FK cascades then remove
  # topics, subscriptions, blackboards, threads, posts, invites, member tokens and
  # inbox rows of the space. Inbox entries the departure hooks produced for that space
  # are not delivered (there is nobody left to receive them).
  #
  # Narrowed: "last leader cannot leave; delete the space instead" applies only while
  # other participants remain (membership.feature:137-140, with a member present). A
  # last leader that is also the last participant may leave, and the space is deleted.
  # Leaderless spaces still arise when an admin revokes the last leader
  # (members or admins remain), or when an admin creates a space.
  #
  # Results are unchanged: leave returns ok, revoke returns {"deleted": true}. The
  # caller learns that the space is gone from bus_list_spaces.

  Background:
    Given the bus was started with an admin token
    And the bootstrap admin minted admin "ops"
    And the bootstrap admin minted leader "lead"

  Rule: Leaving as the last participant deletes the space

    Scenario: An admin that is the only participant leaves
      Given admin "ops" created space "gamma"
      And admin "ops" created topic "notes" and blackboard "b" in space "gamma"
      When admin "ops" leaves space "gamma"
      Then the call succeeds
      And the space "gamma" no longer exists
      And no topic, blackboard, invite or inbox entry of space "gamma" remains

    Scenario: The last member of a leaderless space leaves
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And admin "ops" revoked member "lead" from space "demo"
      When member "worker" leaves space "demo"
      Then the call succeeds
      And the space "demo" no longer exists
      And member "worker"'s token for "demo" fails with unauthenticated "invalid token"

    Scenario: A pending invite does not keep a space alive
      Given admin "ops" created space "gamma"
      And admin "ops" created an invite code for member "late" in space "gamma"
      When admin "ops" leaves space "gamma"
      Then the space "gamma" no longer exists
      And posting the code to /v1/invites/accept fails with unauthenticated "invalid or expired invite code"

    Scenario: A departure that leaves anyone behind keeps the space
      Given admin "ops" created space "gamma"
      And admin "ops" invited member "worker" to space "gamma"
      When admin "ops" leaves space "gamma"
      Then the space "gamma" still exists
      And the participants of space "gamma" are:
        | name   | role   |
        | worker | member |

    Scenario: The lone leader leaves and the space is deleted
      Given leader "lead" created space "solo"
      When leader "lead" leaves space "solo"
      Then the call succeeds
      And the space "solo" no longer exists
      And the token of leader "lead" still authenticates

    Scenario: The last leader still cannot leave while others remain
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When leader "lead" leaves space "demo"
      Then the call fails with conflict "last leader cannot leave; delete the space instead"
      And the space "demo" still exists

  Rule: Revoking the last participant deletes the space

    Scenario: An admin revokes the last leader who was alone
      Given leader "lead" created space "solo"
      When admin "ops" revokes member "lead" from space "solo"
      Then the call returns {"deleted": true}
      And the space "solo" no longer exists
      And the token of leader "lead" still authenticates

    Scenario: An admin revokes the last member of a leaderless space
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And admin "ops" revoked member "lead" from space "demo"
      When admin "ops" revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}
      And the space "demo" no longer exists

    Scenario: A revoke that removes only an invite code keeps an empty-of-others space intact
      # Nothing departs, so the invariant is not checked; the space still has its participants.
      Given leader "lead" created space "demo"
      And leader "lead" created an invite code for member "late" in space "demo"
      When admin "ops" revokes member "late" from space "demo"
      Then the call returns {"deleted": true}
      And the space "demo" still exists

  Rule: Revoking an admin deletes every space it was the last participant of

    Scenario: bus_revoke_admin empties and deletes the admin's solo spaces
      Given admin "ops" created space "gamma"
      And leader "lead" created space "demo"
      And admin "ops" joined space "demo"
      When the bootstrap admin calls bus_revoke_admin with {"name":"ops"}
      Then the call returns {"deleted": true}
      And the space "gamma" no longer exists
      And the space "demo" still exists
