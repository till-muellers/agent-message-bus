Feature: Space overview (bus_space_overview)
  As an agent that just joined a space, or one that lost track of it
  I want one call that shows who is in the space and what is going on there
  So that I can orient myself without calling four list tools and reading every board

  # Moved here from docs/design/specs/guidance/space-overview.feature (rules :33, :97, :135).
  # Served from the one tool registry, so MCP, RPC and the omp extension get it alike.
  # Read-only composition of existing feature reads: spaces (participants), messaging
  # (topics with the caller's subscription state), blackboards (boards, and threads across
  # all boards). It owns no table: `server/internal/overview` depends on the three services
  # through consumer-side interfaces; blackboards has one repository query for the most
  # recent threads of a space.
  #
  # Result:
  #   {space, you: {name, role},                           role null for an admin that has not joined
  #    participants: [{name, role}],                        admins, then leaders, then members; each by name
  #    topics: [{name, createdBy, subscribed}],             open topics, all first, then by name
  #    blackboards: [{name, owner, threads}],               by name
  #    recentThreads: [{id, blackboard, title, createdBy, posts, lastPostAt, subscribed}]}
  #                                                         at most 10, most recent post first
  # Description (purpose first, then access):
  #   "Shows one space at a glance: you and your role, the participants, open topics with your
  #    subscriptions, blackboards and the 10 most recently active threads. Participants and
  #    admins; no side effects."

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" created space "beta"
    And leader "lead" invited member "peer" to space "demo"

  Rule: The overview shows the caller, the participants, topics, boards and recent threads

    Scenario: A member gets the overview of its space
      Given leader "lead" created topic "build" in space "demo"
      And member "worker" subscribed to topic "build" in space "demo"
      And leader "lead" created blackboard "plan" in space "demo"
      And leader "lead" created thread "design" on blackboard "plan" in space "demo"
      When member "worker" calls bus_space_overview with {"space":"demo"}
      Then the result has space "demo" and you {"name":"worker","role":"member"}
      And participants are:
        | name   | role   |
        | lead   | leader |
        | peer   | member |
        | worker | member |
      And topics are:
        | name  | createdBy | subscribed |
        | all   | lead      | true       |
        | build | lead      | true       |
      And blackboards are:
        | name | owner | threads |
        | plan | lead  | 1       |
      And recentThreads holds thread "design" on blackboard "plan" created by "lead" with 1 post and subscribed false

    Scenario: The all topic always shows as subscribed
      When member "peer" calls bus_space_overview with {"space":"demo"}
      Then topics are:
        | name | createdBy | subscribed |
        | all  | lead      | true       |

    Scenario: Recent threads span all boards, newest activity first, at most 10
      Given leader "lead" created blackboard "plan" in space "demo"
      And member "worker" created blackboard "notes" in space "demo"
      And 12 threads were created alternately on "plan" and "notes" in space "demo"
      And leader "lead" added a post to the oldest thread
      When leader "lead" calls bus_space_overview with {"space":"demo"}
      Then recentThreads holds 10 threads ordered by lastPostAt, newest first
      And the first of them is the oldest thread

    Scenario: Thread subscription state is the caller's
      Given leader "lead" created blackboard "plan" in space "demo"
      And member "worker" created thread "t1" on blackboard "plan" in space "demo"
      When member "worker" calls bus_space_overview with {"space":"demo"}
      Then recentThreads holds thread "t1" with subscribed true
      When member "peer" calls bus_space_overview with {"space":"demo"}
      Then recentThreads holds thread "t1" with subscribed false

    Scenario: An empty space shows empty lists, never null
      Given leader "lead" created space "empty"
      When leader "lead" calls bus_space_overview with {"space":"empty"}
      Then blackboards is [] and recentThreads is []
      And participants are:
        | name | role   |
        | lead | leader |

    Scenario: Closed topics are not shown
      Given leader "lead" created topic "old" in space "demo"
      And leader "lead" closed topic "old" in space "demo"
      When leader "lead" calls bus_space_overview with {"space":"demo"}
      Then topics do not include "old"

    Scenario: The overview has no side effects
      When member "worker" calls bus_space_overview with {"space":"demo"}
      Then no subscription, inbox entry or other row has changed

  Rule: The overview passes the space access gate

    Scenario Outline: Access (<case>)
      When <caller> calls bus_space_overview with {"space":"<space>"}
      Then the call fails with <code> "<message>"

      Examples:
        | case                         | caller          | space | code      | message                           |
        | unknown space                | leader "lead"   | nope  | not_found | space nope not found              |
        | member token of another space| member "worker" | beta  | forbidden | token is not valid for space beta |

    Scenario: A leader that does not participate is rejected
      Given the admin minted leader "lead2"
      When leader "lead2" calls bus_space_overview with {"space":"demo"}
      Then the call fails with forbidden "not a participant of space demo"

    Scenario: An admin that joined sees its admin role
      # PO ruling (round 4; docs/design/specs/admin/): admins lead every space without joining; the
      # participant role `admin` exists only where the admin joined.
      Given the admin minted admin "ops"
      And admin "ops" joined space "demo"
      When admin "ops" calls bus_space_overview with {"space":"demo"}
      Then the result has you {"name":"ops","role":"admin"}
      And participants list "ops" with role "admin" first

    Scenario: An admin that has not joined sees the space without a role
      Given the admin minted admin "ops"
      When admin "ops" calls bus_space_overview with {"space":"demo"}
      Then the result has space "demo" and you {"name":"ops","role":null}
      And participants do not include "ops"
      And every topic shows subscribed false, "all" included
      # An admin that has not joined gets no `all` messages (admin-messaging.feature), so "all" is
      # subscribed false for it and true for every participant.

    Scenario: space is required
      When member "worker" calls bus_space_overview with {}
      Then the call fails with invalid "space is required"

  Rule: The tool is listed on every transport

    Scenario: The registry lists the tool
      # Final registry after every proposal: 34 server tools (docs/design/specs/REVIEW.md §11 lists the
      # count at each merge step).
      When the admin calls GET /v1/tools
      Then the response contains "bus_space_overview"
      And server/TOOLS.md lists bus_space_overview in the group "Spaces & membership"
