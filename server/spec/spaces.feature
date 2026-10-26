Feature: Spaces
  As a leader agent
  I want to create, join, list and delete spaces and see who participates in them
  So that each team of agents works in its own isolated space

  # A space has participants with role "leader" or "member". Leader agents may
  # participate in many spaces; a member token is valid for exactly one space.
  # Every space-scoped operation passes the same authorization gate (see the
  # "Space access gate" rule); its errors are listed once here.

  Background:
    Given the admin minted leader "lead"
    And the admin minted leader "lead2"

  Rule: Space access gate for every space-scoped operation

    # An admin passes the gate in every space without joining it (admin-role.feature).

    Scenario: An unknown space is not found
      When leader "lead" lists the participants of space "nope"
      Then the call fails with not_found "space nope not found"

    Scenario: A member token is valid only for the space it was issued for
      Given leader "lead" created space "demo"
      And leader "lead" created space "beta"
      And leader "lead" invited member "worker" to space "demo"
      When member "worker" lists the participants of space "beta"
      Then the call fails with forbidden "token is not valid for space beta"

    Scenario: A member token stays bound to its space even if the agent was also invited elsewhere
      Given leader "lead" created space "demo"
      And leader "lead" created space "beta"
      And leader "lead" invited member "worker" to space "demo"
      And leader "lead" invited member "worker" to space "beta"
      When member "worker" uses its "demo" token to list the participants of space "beta"
      Then the call fails with forbidden "token is not valid for space beta"

    Scenario: A leader that is not a participant is rejected
      Given leader "lead" created space "demo"
      When leader "lead2" lists the participants of space "demo"
      Then the call fails with forbidden "not a participant of space demo"

    Scenario Outline: The gate resolves the caller's role in the space (<caller>)
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When <caller> lists the participants of space "demo"
      Then the call succeeds
      And the caller acts with role "<role>" in space "demo"

      Examples:
        | caller          | role   |
        | leader "lead"   | leader |
        | member "worker" | member |

  Rule: Leaders create spaces (bus_create_space)

    Scenario: A leader creates a space and becomes its leader
      When leader "lead" creates space "demo"
      Then the call succeeds
      And space "demo" has an open topic "all" created by "lead"
      And the participants of space "demo" are:
        | name | role   |
        | lead | leader |

    Scenario: Space names are unique
      Given leader "lead" created space "demo"
      When leader "lead2" creates space "demo"
      Then the call fails with conflict "space demo already exists"

    Scenario Outline: Members cannot create spaces (<caller>)
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When <caller> creates space "gamma"
      Then the call fails with forbidden "only leaders may create spaces"

      Examples:
        | caller          |
        | member "worker" |

    Scenario Outline: Space names must be valid (<case>)
      When leader "lead" creates space "<space>"
      Then the call fails with invalid "space must match ^[a-z0-9][a-z0-9_-]{0,62}$"

      Examples:
        | case         | space |
        | empty        |       |
        | uppercase    | Demo  |
        | leading dash | -demo |

    Scenario: The space argument is required
      When leader "lead" creates a space without giving a space name
      Then the call fails with invalid "space is required"

  Rule: Leaders join existing spaces as leaders (bus_join_space)

    Scenario: A leader joins an existing space
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When leader "lead2" joins space "demo"
      Then the call succeeds
      And the participants of space "demo" are:
        | name   | role   |
        | lead   | leader |
        | lead2  | leader |
        | worker | member |

    Scenario: A participant cannot join again
      Given leader "lead" created space "demo"
      When leader "lead" joins space "demo"
      Then the call fails with conflict "already a participant of demo"

    Scenario Outline: Members cannot join spaces (<caller>)
      Given leader "lead" created space "demo"
      And leader "lead" invited member "worker" to space "demo"
      When <caller> joins space "demo"
      Then the call fails with forbidden "only leaders may join spaces"

      Examples:
        | caller          |
        | member "worker" |

    Scenario: Joining an unknown space fails
      When leader "lead2" joins space "nope"
      Then the call fails with not_found "space nope not found"

  Rule: Listing spaces (bus_list_spaces)

    Scenario: A leader sees every space, ordered by name
      Given leader "lead2" created space "beta"
      And leader "lead" created space "alpha"
      And leader "lead" invited member "worker" to space "alpha"
      When leader "lead" lists spaces
      Then the call returns:
        | name  | leaders | members | joined |
        | alpha | lead    | 1       | true   |
        | beta  | lead2   | 0       | false  |

    Scenario: A member sees only its own space
      Given leader "lead2" created space "beta"
      And leader "lead" created space "alpha"
      And leader "lead" invited member "worker" to space "alpha"
      When member "worker" lists spaces
      Then the call returns:
        | name  | leaders | members | joined |
        | alpha | lead    | 1       | true   |

    Scenario: A leader with no spaces gets an empty list
      When leader "lead" lists spaces
      Then the call returns an empty list

    Scenario: The admin lists every space
      When the admin lists spaces
      Then the call returns every space with "joined" false where the admin has not joined

    Scenario: Listing spaces takes no arguments
      When leader "lead" lists spaces with the extra argument "x"
      Then the call fails with invalid "invalid arguments: json: unknown field \"x\""

  Rule: Listing participants (bus_list_participants)

    Scenario: Participants are listed leaders first, then by name
      Given the admin minted leader "zed"
      And the admin minted leader "amy"
      And leader "zed" created space "alpha"
      And leader "amy" joined space "alpha"
      And leader "zed" invited member "bob" to space "alpha"
      And leader "zed" invited member "abe" to space "alpha"
      When member "bob" lists the participants of space "alpha"
      Then the call returns:
        | name | role   |
        | amy  | leader |
        | zed  | leader |
        | abe  | member |
        | bob  | member |

    Scenario: The admin lists the participants of a space it has not joined
      Given leader "lead" created space "alpha"
      When the admin lists the participants of space "alpha"
      Then the call returns:
        | name | role   |
        | lead | leader |

  Rule: Identity (bus_whoami)

    Scenario: The admin is the bootstrap admin "admin" with no spaces
      When the admin asks who it is
      Then the call returns kind "admin", name "admin" and spaces []

    Scenario: A leader sees its name and every space it participates in, ordered by name
      Given leader "lead" created space "beta"
      And leader "lead" created space "alpha"
      When leader "lead" asks who it is
      Then the call returns kind "leader" and name "lead"
      And the call returns spaces:
        | name  | role   |
        | alpha | leader |
        | beta  | leader |

    Scenario: A member also sees the space its token is bound to
      Given leader "lead" created space "beta"
      And leader "lead" created space "alpha"
      And leader "lead" invited member "worker" to space "alpha"
      When member "worker" asks who it is
      Then the call returns kind "member", name "worker" and space "alpha"
      And the call returns spaces:
        | name  | role   |
        | alpha | member |

  Rule: Deleting a space (bus_delete_space)

    Background:
      Given the admin minted leader "stranger"
      And leader "lead" created space "alpha"
      And leader "lead" invited member "worker" to space "alpha"
      And leader "lead2" joined space "alpha"

    Scenario: A member cannot delete the space
      When member "worker" deletes space "alpha"
      Then the call fails with forbidden "only leaders may delete spaces"

    Scenario: A leader that is not a participant cannot delete the space
      When leader "stranger" deletes space "alpha"
      Then the call fails with forbidden "not a participant of space alpha"

    Scenario: The admin deletes a space it has not joined
      When the admin deletes space "alpha"
      Then the call succeeds

    Scenario: A space with other leaders cannot be deleted
      When leader "lead" deletes space "alpha"
      Then the call fails with conflict "space alpha has other leaders; they must leave first"

    Scenario: The sole remaining leader deletes the space once the others have left
      Given leader "lead2" left space "alpha"
      When leader "lead" deletes space "alpha"
      Then the call succeeds
      When leader "lead" deletes space "alpha"
      Then the call fails with not_found "space alpha not found"

    Scenario: Deleting a space removes everything in it, including member tokens
      Given leader "lead2" left space "alpha"
      And space "alpha" has topics, blackboards with threads and posts, subscriptions and inbox entries
      When leader "lead" deletes space "alpha"
      Then the call succeeds
      And space "alpha" no longer has participants, topics, subscriptions, blackboards, threads, posts or inbox entries
      When member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

    Scenario: Leader tokens and agent identities outlive a deleted space
      Given leader "lead2" left space "alpha"
      When leader "lead" deletes space "alpha"
      Then leader "lead" can still list spaces
      When the admin mints leader "worker"
      Then the call fails with conflict "agent worker is a member agent"
