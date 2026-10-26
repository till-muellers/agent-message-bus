Feature: The admin is a role that leads every space
  As the operator of a bus
  I want admin agents that act as a leader in every space without joining it
  So that one credential can inspect, steer and clean up the whole instance

  # PROPOSAL (docs/design/specs/admin/analysis.md). Supersedes docs/design/specs/web/admin-superuser.feature.
  #
  # PO ruling 1: `admin` is a real agent kind next to `leader` and `member`. An admin
  # agent has a name (^[a-z0-9][a-z0-9_-]{0,62}$) and its own inbox. It uses the
  # administration tools and acts as a leader in every space: read, write, post,
  # critical, `all`, delete, revoke, invite. It does not have to join a space for that.
  #
  # Authority vs participation (analysis.md §4.1, seam B):
  #   - authority: every admin "leads" every space. All leader checks pass for it.
  #   - participation: an admin is a participant only of spaces it created or joined.
  #     Then it has a participant row with role "admin" (admin-messaging.feature).
  #     Participation decides who can message it, not what it may do.
  #
  # The bootstrap admin is the agent "admin" behind the instance admin token
  # (admin-tokens.feature). Every admin token, bootstrap or minted, behaves the
  # same in spaces. Scenarios below use the minted admin "ops"; they hold for the
  # bootstrap admin "admin" as well.
  #
  # Admins are a strict SUPERSET of the leader role (PO ruling):
  #   - an admin deletes any space, even one with leaders;
  #   - an admin revokes members AND leaders in every space, the last leader included.
  #     Revoking a leader removes its participation in that space (departure hooks,
  #     its inbox entries there). Its leader tokens are not space-bound and stay valid.
  #     A space whose last leader was revoked keeps its other participants and has no
  #     leader. Admins still lead it and any leader may join it. If nobody remains,
  #     the space is deleted (no-empty-spaces.feature).
  #   - an admin is never revoked from a space with bus_revoke_member; admins are
  #     removed with bus_revoke_admin (admin-tokens.feature) or leave by themselves.
  #
  # Same behaviour over POST /v1/tools/{name}, POST /mcp and the omp extension.

  Background:
    Given the bus was started with an admin token
    And the bootstrap admin minted admin "ops"
    And the bootstrap admin minted leader "lead"
    And the bootstrap admin minted leader "lead2"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead2" created space "other"

  Rule: An admin reads every space without joining it

    Scenario: An admin lists every space
      When admin "ops" lists spaces
      Then the call returns:
        | name  | leaders | admins | members | joined |
        | demo  | lead    |        | 1       | false  |
        | other | lead2   |        | 0       | false  |

    Scenario: A leader's and a member's space lists are unchanged apart from the admins column
      When member "worker" lists spaces
      Then the call returns:
        | name | leaders | admins | members | joined |
        | demo | lead    |        | 1       | true   |

    Scenario: An admin lists the participants of a space it has not joined
      When admin "ops" lists the participants of space "demo"
      Then the call returns:
        | name   | role   |
        | lead   | leader |
        | worker | member |

    Scenario Outline: An admin uses the read tools of any space (<tool>)
      Given leader "lead" created topic "notes" in space "demo"
      And member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      When admin "ops" calls <tool> with <args>
      Then the call succeeds

      Examples:
        | tool                  | args                                       |
        | bus_list_topics       | {"space":"demo"}                           |
        | bus_list_blackboards  | {"space":"demo"}                           |
        | bus_list_threads      | {"space":"demo","blackboard":"b"}          |
        | bus_read_thread       | {"space":"demo","threadId":1}              |

    Scenario: An admin that has not joined is never subscribed
      Given leader "lead" created topic "notes" in space "demo"
      When admin "ops" lists the topics of space "demo"
      Then the call returns:
        | name  | createdBy | subscribed |
        | all   | lead      | false      |
        | notes | lead      | false      |

    Scenario: An unknown space is still not found
      When admin "ops" lists the participants of space "ghost"
      Then the call fails with not_found "space ghost not found"

  Rule: An admin sends and posts with leader authority everywhere

    Scenario: An admin sends a direct message into a space it has not joined
      When admin "ops" sends direct message "hi" to "worker" in space "demo"
      Then the call succeeds
      And member "worker" receives a "direct_message" inbox entry from "ops" with body "hi"

    Scenario: An admin sends a critical direct message
      When admin "ops" sends the critical direct message "stop" to "worker" in space "demo"
      Then the call succeeds
      And member "worker" receives a critical inbox entry from "ops"

    Scenario: An admin sends to the all topic
      When admin "ops" sends the topic message "hi" to topic "all" in space "demo"
      Then the call succeeds
      And leader "lead" receives a "topic_message" inbox entry from "ops" with topic "all"
      And member "worker" receives a "topic_message" inbox entry from "ops" with topic "all"

    Scenario: An admin sends to a topic
      Given leader "lead" created topic "notes" in space "demo"
      And member "worker" subscribed to topic "notes" in space "demo"
      When admin "ops" sends the topic message "hi" to topic "notes" in space "demo"
      Then member "worker" receives a "topic_message" inbox entry from "ops" with topic "notes" and body "hi"

    Scenario: An admin creates a topic, a blackboard and a thread
      When admin "ops" creates topic "ops-notes" in space "demo"
      And admin "ops" creates blackboard "audit" in space "demo"
      And admin "ops" creates thread "findings" with body "first" on blackboard "audit" in space "demo"
      Then the topic "ops-notes" of space "demo" was created by "ops"
      And the blackboard "audit" of space "demo" has owner "ops"
      And thread 1 has author "ops"

    Scenario: An admin that has not joined is not subscribed to what it creates
      # Subscriptions feed an inbox. An admin gets mail only where it participates.
      When admin "ops" creates topic "ops-notes" in space "demo"
      And member "worker" created blackboard "b" in space "demo"
      And admin "ops" creates thread "plan" with body "first" on blackboard "b" in space "demo"
      Then admin "ops" is not subscribed to topic "ops-notes" in space "demo"
      And admin "ops" is not subscribed to thread 1 in space "demo"

    Scenario: An admin posts to a thread of any space
      Given member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      When admin "ops" posts "reply" to thread 1 in space "demo"
      Then the call succeeds
      And member "worker" receives a "thread_post" inbox entry from "ops" with threadId 1

  Rule: An admin removes and closes like a leader

    Scenario: An admin closes a topic it did not create
      Given leader "lead" created topic "notes" in space "demo"
      When admin "ops" closes topic "notes" in space "demo"
      Then the call succeeds

    Scenario: An admin removes a thread it did not create
      Given member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      When admin "ops" removes thread 1 in space "demo"
      Then the call succeeds

    Scenario: An admin deletes a blackboard it does not own
      Given member "worker" created blackboard "b" in space "demo"
      When admin "ops" deletes blackboard "b" in space "demo"
      Then the call succeeds

  Rule: An admin invites and revokes members and leaders in every space

    Scenario: An admin invites a member into a space it has not joined
      When admin "ops" invites member "helper" to space "demo"
      Then the call returns name "helper", space "demo" and an invite code
      And the code is accepted

    Scenario: An admin revokes a member
      When admin "ops" revokes member "worker" from space "demo"
      Then the call returns {"deleted": true}

    Scenario: An admin revokes a leader
      Given leader "lead2" joined space "demo"
      When admin "ops" revokes member "lead2" from space "demo"
      Then the call returns {"deleted": true}
      And leader "lead2" is not a participant of space "demo"
      And the token of leader "lead2" still authenticates

    Scenario: An admin revokes the last leader of a space
      When admin "ops" revokes member "lead" from space "demo"
      Then the call returns {"deleted": true}
      And the participants of space "demo" are:
        | name   | role   |
        | worker | member |
      And space "demo" still exists with leaders []
      And leader "lead" has no inbox entry in space "demo"
      And leader "lead" may join space "demo" again

    Scenario: A leader still cannot revoke a leader
      Given leader "lead2" joined space "demo"
      When leader "lead" revokes member "lead2" from space "demo"
      Then the call fails with conflict "lead2 is a leader of space demo; leaders cannot be revoked"

    Scenario: An admin cannot revoke an admin with bus_revoke_member
      Given admin "ops" joined space "demo"
      And the bootstrap admin minted admin "web"
      When admin "web" revokes member "ops" from space "demo"
      Then the call fails with conflict "ops is an admin of space demo; admins cannot be revoked from a space"

    Scenario: Nobody revokes an admin from a space with bus_revoke_member
      Given admin "ops" joined space "demo"
      When leader "lead" revokes member "ops" from space "demo"
      Then the call fails with conflict "ops is an admin of space demo; admins cannot be revoked from a space"

    Scenario: An admin name cannot be invited as a member
      When leader "lead" invites member "ops" to space "demo"
      Then the call fails with conflict "agent ops is an admin agent"

  Rule: An admin creates and deletes spaces

    Scenario: An admin creates a space and participates in it as admin
      When admin "ops" creates space "gamma"
      Then the call succeeds
      And the participants of space "gamma" are:
        | name | role  |
        | ops  | admin |
      And the topic "all" of space "gamma" was created by "ops"

    Scenario: An admin deletes a space that has leaders
      When admin "ops" deletes space "demo"
      Then the call succeeds
      And the space "demo" no longer exists

    Scenario: A leader still cannot delete a space that has other leaders
      Given leader "lead2" joined space "demo"
      When leader "lead" deletes space "demo"
      Then the call fails with conflict "space demo has other leaders; they must leave first"

    Scenario: A joined admin does not count as another leader
      Given admin "ops" joined space "demo"
      When leader "lead" deletes space "demo"
      Then the call succeeds

  Rule: Admin authority does not leak to other kinds

    Scenario Outline: A leader who is not a participant is still refused (<action>)
      When leader "lead2" <action> in space "demo"
      Then the call fails with forbidden "not a participant of space demo"

      Examples:
        | action                               |
        | lists the participants               |
        | sends direct message "x" to "worker" |

    Scenario: A member still cannot send critical messages
      When member "worker" sends the critical direct message "x" to "lead" in space "demo"
      Then the call fails with forbidden "only leaders may send critical messages"

  Rule: The admin role is the same over every transport

    Scenario: The JSON web API behaves like the tool call
      When admin "ops" calls POST /v1/tools/bus_list_participants with {"space":"demo"}
      Then the response status is 200

    Scenario: MCP runs the call as the admin agent
      When a client connects to /mcp with the token of admin "ops"
      Then its tool calls run as admin "ops"
      And the MCP user id is "admin:<agent id of ops>"
