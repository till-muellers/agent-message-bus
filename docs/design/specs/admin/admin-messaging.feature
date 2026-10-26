Feature: An admin is reachable only in the spaces it joined
  As an agent working in a space
  I want to message an admin that takes part in my space
  So that an operator who joins a space can be talked to like any participant

  # PROPOSAL (docs/design/specs/admin/analysis.md). PO ruling 2.
  #
  # Every admin agent has its own inbox (bus_inbox_read, bus_inbox_ack,
  # GET /v1/inbox/stream). Like a leader's, it spans every space.
  #
  # Joining (bus_join_space, or creating the space) gives the admin a participant
  # row with role "admin". "admin" is a third participant role next to "leader"
  # and "member". The row is what makes the admin reachable:
  #   - it can be addressed by bus_send_direct in that space;
  #   - it is a recipient of the `all` topic of that space;
  #   - it may subscribe to topics and threads of that space, and it is subscribed
  #     to the topics and threads it creates there.
  # An admin row is not a leader: it does not count in "leaders", it neither blocks
  # nor allows "the last leader cannot leave", and it does not stop a leader from
  # deleting the space.
  #
  # Invariant: an admin's inbox only ever holds entries of spaces it participates in.
  # That is why an admin that has not joined cannot subscribe, and why leaving
  # removes its subscriptions and entries of that space (the existing departure path).
  #
  # Leaving (bus_leave_space) removes the row the same way a leader's departure does:
  # departure hooks (topic and thread subscriptions; blackboards stay because the
  # role is not "member"), then the admin's inbox entries of that space. Admin
  # tokens are not bound to a space, so no token is deleted. The admin keeps its
  # authority in that space. It just cannot be reached there any more.

  Background:
    Given the bus was started with an admin token
    And the bootstrap admin minted admin "ops"
    And the bootstrap admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" created space "other"

  Rule: An admin that has not joined cannot be messaged

    Scenario: A direct message to an admin that has not joined fails
      When member "worker" sends direct message "hello" to "ops" in space "demo"
      Then the call fails with not_found "participant ops not in space demo"

    Scenario: A reply to an admin's message fails while the admin has not joined
      Given admin "ops" sent direct message "status?" to "worker" in space "demo"
      When member "worker" sends direct message "done" to "ops" in space "demo"
      Then the call fails with not_found "participant ops not in space demo"

    Scenario: An admin that has not joined gets nothing from the all topic
      When leader "lead" sends the topic message "hi" to topic "all" in space "demo"
      Then admin "ops" has no inbox entry

    Scenario Outline: An admin that has not joined cannot subscribe (<what>)
      Given leader "lead" created topic "notes" in space "demo"
      And leader "lead" created blackboard "b" in space "demo"
      And leader "lead" created thread "plan" with body "first" on blackboard "b" in space "demo"
      When admin "ops" <action> in space "demo"
      Then the call fails with forbidden "join space demo first; only participants receive messages"

      Examples:
        | what                | action                             |
        | topic subscribe     | subscribes to topic "notes"        |
        | topic unsubscribe   | unsubscribes from topic "notes"    |
        | thread subscribe    | subscribes to thread 1             |
        | thread unsubscribe  | unsubscribes from thread 1         |

  Rule: Joining a space makes the admin a participant with role "admin"

    Scenario: An admin joins a space
      When admin "ops" joins space "demo"
      Then the call succeeds
      And the participants of space "demo" are:
        | name   | role   |
        | ops    | admin  |
        | lead   | leader |
        | worker | member |

    Scenario: Participants are listed admins first, then leaders, then members, each by name
      Given the bootstrap admin joined space "demo"
      And admin "ops" joined space "demo"
      When leader "lead" lists the participants of space "demo"
      Then the call returns:
        | name   | role   |
        | admin  | admin  |
        | ops    | admin  |
        | lead   | leader |
        | worker | member |

    Scenario: The space list names joined admins separately from leaders and members
      Given admin "ops" joined space "demo"
      When leader "lead" lists spaces
      Then the call returns:
        | name  | leaders | admins | members | joined |
        | demo  | lead    | ops    | 1       | true   |
        | other | lead    |        | 0       | true   |

    Scenario: Joining twice is a conflict
      Given admin "ops" joined space "demo"
      When admin "ops" joins space "demo"
      Then the call fails with conflict "already a participant of demo"

    Scenario: Joining an unknown space fails
      When admin "ops" joins space "ghost"
      Then the call fails with not_found "space ghost not found"

    Scenario: whoami lists the spaces an admin joined
      Given admin "ops" joined space "demo"
      When admin "ops" asks who it is
      Then the call returns kind "admin", name "ops" and spaces:
        | name | role  |
        | demo | admin |

    Scenario: An admin that joined nothing has no spaces in whoami
      When admin "ops" asks who it is
      Then the call returns kind "admin", name "ops" and spaces []

  Rule: A joined admin is messaged like any participant

    Scenario: A member messages a joined admin
      Given admin "ops" joined space "demo"
      When member "worker" sends direct message "need a decision" to "ops" in space "demo"
      Then the call succeeds
      And admin "ops" has a "direct_message" inbox entry from "worker" in space "demo" with body "need a decision"

    Scenario: A joined admin gets the all topic
      Given admin "ops" joined space "demo"
      When leader "lead" sends the topic message "release at 5" to topic "all" in space "demo"
      Then admin "ops" has a "topic_message" inbox entry from "lead" with topic "all"

    Scenario: A joined admin subscribes to a topic and gets its messages
      Given admin "ops" joined space "demo"
      And leader "lead" created topic "notes" in space "demo"
      When admin "ops" subscribes to topic "notes" in space "demo"
      And member "worker" sends the topic message "found it" to topic "notes" in space "demo"
      Then admin "ops" has a "topic_message" inbox entry from "worker" with topic "notes"

    Scenario: A joined admin is subscribed to the thread it creates
      Given admin "ops" joined space "demo"
      And leader "lead" created blackboard "b" in space "demo"
      When admin "ops" creates thread "plan" with body "first" on blackboard "b" in space "demo"
      And member "worker" posts "reply" to thread 1 in space "demo"
      Then admin "ops" has a "thread_post" inbox entry from "worker" with threadId 1

    Scenario: Joining one space does not make the admin reachable in another
      Given admin "ops" joined space "demo"
      When leader "lead" sends direct message "x" to "ops" in space "other"
      Then the call fails with not_found "participant ops not in space other"

  Rule: An admin reads its own inbox across every space it joined

    Scenario: An admin reads and acknowledges its inbox
      Given admin "ops" joined space "demo"
      And admin "ops" joined space "other"
      And member "worker" sent direct message "a" to "ops" in space "demo"
      And leader "lead" sent direct message "b" to "ops" in space "other"
      When admin "ops" reads its inbox
      Then the entries are "a" from space "demo" and "b" from space "other", in id order
      When admin "ops" acknowledges its inbox up to the id of entry "b"
      Then the call returns {"acked": 2}

    Scenario: An admin opens its inbox stream
      Given admin "ops" joined space "demo"
      When admin "ops" opens the inbox stream
      Then the response status is 200
      And the stream sends the backlog of admin "ops"

    Scenario: An admin has one stream slot like any agent
      Given admin "ops" opened the inbox stream
      When admin "ops" opens the inbox stream again with another of its tokens
      Then the response status is 409
      And the call fails with conflict "agent ops already has an active inbox stream"

    Scenario: Two admins have separate inboxes and slots
      Given admin "ops" opened the inbox stream
      When the bootstrap admin opens the inbox stream
      Then the response status is 200

  Rule: Leaving removes the admin's participation, not its authority

    Scenario: An admin leaves a space
      Given admin "ops" joined space "demo"
      And leader "lead" created topic "notes" in space "demo"
      And admin "ops" subscribed to topic "notes" in space "demo"
      And member "worker" sent direct message "a" to "ops" in space "demo"
      When admin "ops" leaves space "demo"
      Then the call succeeds
      And admin "ops" is not a participant of space "demo"
      And admin "ops" is not subscribed to topic "notes" in space "demo"
      And admin "ops" has no inbox entry in space "demo"
      And the token of admin "ops" still authenticates

    Scenario: After leaving, the admin cannot be messaged there but still acts there
      Given admin "ops" joined space "demo"
      And admin "ops" left space "demo"
      When member "worker" sends direct message "x" to "ops" in space "demo"
      Then the call fails with not_found "participant ops not in space demo"
      When admin "ops" sends direct message "x" to "worker" in space "demo"
      Then the call succeeds

    Scenario: An admin's blackboards stay when it leaves
      Given admin "ops" joined space "demo"
      And admin "ops" created blackboard "audit" in space "demo"
      When admin "ops" leaves space "demo"
      Then the blackboard "audit" of space "demo" still exists with owner "ops"

    Scenario: The last leader rule ignores admins
      Given admin "ops" joined space "demo"
      When leader "lead" leaves space "demo"
      Then the call fails with conflict "last leader cannot leave; delete the space instead"

    Scenario: An admin that is the only participant leaves and the space is deleted
      # No empty spaces (no-empty-spaces.feature).
      Given admin "ops" created space "gamma"
      When admin "ops" leaves space "gamma"
      Then the call succeeds
      And the space "gamma" no longer exists

    Scenario: An admin that has not joined cannot leave
      When admin "ops" leaves space "demo"
      Then the call fails with forbidden "not a participant of space demo"
