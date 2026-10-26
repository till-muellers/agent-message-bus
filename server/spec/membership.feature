Feature: Space membership
  As a leader of a space
  I want to invite and revoke member agents and let participants leave
  So that only the agents I choose can act in my space, and departures leave no residue

  Background:
    Given the admin minted leader "lead"
    And the admin minted leader "lead2"
    And leader "lead" created space "demo"

  Rule: Leaders of a space invite members with invite codes (bus_invite_member)

    # "leader x invited member y to space z" means: x created an invite code that y accepted.
    # Code lifetime and redemption are specified in invites.feature.

    Scenario: A leader invites a new member and the member accepts the code
      When leader "lead" invites member "worker" to space "demo"
      Then the call returns name "worker", space "demo" and an invite code
      When the code is accepted
      Then the returned token authenticates as member "worker" bound to space "demo"
      And the participants of space "demo" are:
        | name   | role   |
        | lead   | leader |
        | worker | member |

    Scenario: Re-inviting a member yields a second working token without duplicating the participant
      Given leader "lead" invited member "worker" to space "demo"
      When leader "lead" invites member "worker" to space "demo" again
      And the code is accepted
      Then the returned token is different from the first one
      And it authenticates as the same member "worker" in space "demo"
      And the first token still authenticates as member "worker"
      And "worker" appears exactly once among the participants of space "demo"

    Scenario: A member agent can be invited into another space with a token for that space
      Given leader "lead" created space "beta"
      And leader "lead" invited member "worker" to space "demo"
      When leader "lead" invites member "worker" to space "beta"
      And the code is accepted
      Then the returned token authenticates as member "worker" bound to space "beta"
      And the earlier token stays bound to space "demo"

    Scenario Outline: Only leaders of the space may invite (<case>)
      Given leader "lead" invited member "worker" to space "demo"
      When <caller> invites member "x" to space "<space>"
      Then the call fails with <code> "<message>"

      Examples:
        | case                   | caller          | space | code      | message                                                               |
        | member caller          | member "worker" | demo  | forbidden | only leaders may invite members                                       |
        | leader not participant | leader "lead2"  | demo  | forbidden | not a participant of space demo                                       |
        | unknown space          | leader "lead"   | nope  | not_found | space nope not found                                                  |

    Scenario: The invitee name must be a member agent, not a leader agent
      When leader "lead" invites member "lead2" to space "demo"
      Then the call fails with conflict "agent lead2 is a leader agent"

    Scenario Outline: Invitee names must be valid (<case>)
      When leader "lead" invites member "<name>" to space "demo"
      Then the call fails with invalid "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"

      Examples:
        | case             | name   |
        | uppercase        | Worker |
        | leading dash     | -w     |
        | contains a space | a b    |

    Scenario Outline: Space and name arguments are required (<missing>)
      When leader "lead" invites a member without giving the <missing> argument
      Then the call fails with invalid "<missing> is required"

      Examples:
        | missing |
        | space   |
        | name    |

  Rule: Leaders of a space revoke members idempotently (bus_revoke_member)

    # Revoking removes the member, if present, and every pending invite code for that name
    # in the space (see invites.feature). It never fails because nothing matched; the result
    # {"deleted": true|false} says whether anything was removed.

    Background:
      Given leader "lead2" joined space "demo"
      And leader "lead" invited member "worker" to space "demo"
      And leader "lead" invited member "peer" to space "demo"

    Scenario: A member cannot revoke another member
      When member "peer" revokes member "worker" from space "demo"
      Then the call fails with forbidden "only leaders may revoke members"

    Scenario: The admin revokes members
      When the admin revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}

    Scenario: An unknown space is not found
      When leader "lead" revokes member "worker" from space "nope"
      Then the call fails with not_found "space nope not found"

    Scenario: A leader cannot be revoked
      When leader "lead" revokes member "lead2" from space "demo"
      Then the call fails with conflict "lead2 is a leader of space demo; leaders cannot be revoked"
      And "lead2" is a leader of space "demo"

    Scenario: Revoking a name with nothing to remove reports deleted false
      When leader "lead" revokes member "ghost" from space "demo"
      Then the call returns {"deleted": false}

    Scenario: Revoking the same member twice reports deleted false the second time
      Given leader "lead" revoked member "worker" from space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then the call returns {"deleted": false}

    Scenario: A leader revokes a member
      Given member "worker" has an unread "direct_message" inbox entry from "lead" in space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}
      And member "worker" has no inbox entries left
      And the participants of space "demo" are:
        | name  | role   |
        | lead  | leader |
        | lead2 | leader |
        | peer  | member |
      When member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

  Rule: Participants leave spaces (bus_leave_space)

    Scenario: A member may always leave
      Given leader "lead" invited member "worker" to space "demo"
      When member "worker" leaves space "demo"
      Then the call succeeds
      When member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

    Scenario: The last leader cannot leave while other participants remain
      Given leader "lead" invited member "worker" to space "demo"
      When leader "lead" leaves space "demo"
      Then the call fails with conflict "last leader cannot leave; delete the space instead"

    Scenario: A leader may leave while another leader remains, keeping its leader token
      Given leader "lead2" joined space "demo"
      When leader "lead2" leaves space "demo"
      Then the call succeeds
      And leader "lead2" can still list spaces
      When leader "lead2" lists the participants of space "demo"
      Then the call fails with forbidden "not a participant of space demo"

    Scenario: An admin that has not joined cannot leave a space
      When the admin leaves space "demo"
      Then the call fails with forbidden "not a participant of space demo"

    Scenario: A lone leader may leave and the space is deleted
      # No empty spaces (no-empty-spaces.feature).
      Given leader "lead2" created space "solo"
      When leader "lead2" leaves space "solo"
      Then the call succeeds
      And the space "solo" no longer exists

  Rule: Departure (leave or revoke) cleans up in a fixed order within one transaction

    # Order: departure hooks run first while the participant still exists
    #   1. messaging: the agent's topic subscriptions in the space are removed
    #   2. blackboards: the agent's thread subscriptions in the space are removed; if the
    #      agent is a member, every blackboard it owns in the space is deleted and each
    #      remaining thread subscriber gets a blackboard_removed entry
    # then the agent's inbox entries for that space, its tokens bound to that space and
    # finally its participant row are deleted. Entries produced by hooks are stored in the
    # same transaction and delivered after it commits; any failure rolls the whole departure back.

    # Threads are referred to by title below for readability; the bus identifies them by numeric id.
    Background:
      Given leader "lead2" joined space "demo"
      And leader "lead" invited member "worker" to space "demo"

    Scenario: The departing agent's topic subscriptions are removed
      Given leader "lead2" created topic "news" in space "demo"
      When leader "lead2" leaves space "demo"
      And leader "lead2" joins space "demo"
      And member "worker" sends "hi" to topic "news" in space "demo"
      Then "lead2" receives no inbox entry

    Scenario: The departing agent's thread subscriptions are removed
      Given leader "lead" created blackboard "plans" in space "demo"
      And leader "lead2" created thread "t1" on blackboard "plans" in space "demo"
      When leader "lead2" leaves space "demo"
      And leader "lead2" joins space "demo"
      And leader "lead" posts "update" to thread "t1" on blackboard "plans" in space "demo"
      Then "lead2" receives no inbox entry

    Scenario: A departing member's blackboards are deleted and remaining thread subscribers are notified
      Given member "worker" created blackboard "notes" in space "demo"
      And member "worker" created thread "t1" on blackboard "notes" in space "demo"
      And leader "lead" subscribed to thread "t1" on blackboard "notes" in space "demo"
      When member "worker" leaves space "demo"
      Then the call succeeds
      And blackboard "notes" no longer exists in space "demo"
      And leader "lead" receives a "blackboard_removed" inbox entry from "worker" with blackboard "notes" in space "demo"
      And "lead2" receives no inbox entry

    Scenario: A revoked member's blackboards are deleted the same way
      Given member "worker" created blackboard "notes" in space "demo"
      And member "worker" created thread "t1" on blackboard "notes" in space "demo"
      And leader "lead2" subscribed to thread "t1" on blackboard "notes" in space "demo"
      When leader "lead" revokes member "worker" from space "demo"
      Then blackboard "notes" no longer exists in space "demo"
      And leader "lead2" receives a "blackboard_removed" inbox entry from "worker" with blackboard "notes" in space "demo"

    Scenario: A departing leader's blackboards persist
      Given leader "lead2" created blackboard "plans" in space "demo"
      When leader "lead2" leaves space "demo"
      Then blackboard "plans" still exists in space "demo" owned by "lead2"
      And no "blackboard_removed" inbox entry is created

    Scenario: Only the departing agent's inbox entries for that space are deleted
      Given leader "lead" created space "beta"
      And leader "lead2" joined space "beta"
      And leader "lead" sent "a" directly to "lead2" in space "demo"
      And leader "lead" sent "b" directly to "lead2" in space "beta"
      When leader "lead2" leaves space "demo"
      Then leader "lead2" still has the inbox entry "b" from space "beta"
      And leader "lead2" has no inbox entry from space "demo"

    Scenario: Only the departing member's token for that space is revoked
      Given leader "lead" created space "beta"
      And leader "lead" invited member "worker" to space "beta"
      When member "worker" leaves space "demo"
      Then the "demo" token of "worker" fails with unauthenticated "invalid token"
      And the "beta" token of "worker" still authenticates as member "worker" bound to space "beta"
