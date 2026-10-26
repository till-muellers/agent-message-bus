Feature: Spaces and participants in the web GUI
  As a human operator of a bus
  I want to see every space and who participates in them
  So that I know which teams are running and who holds which role

  # Moved from docs/design/specs/web/spaces-and-participants.feature (slice W1b).
  #
  # The web GUI operator signs in with an admin token (see
  # ../admin-tokens.feature). An admin acts as a leader in every
  # space without joining it, so the web GUI shows every space and every
  # participant.
  #
  # The web GUI reuses the rules of the feature services:
  #   - roles are "admin", "leader" and "member"
  #   - an admin appears in a participant list only if it joined the space,
  #     and admins are listed first, then leaders, then members, each by name
  # These views are read-only; the milestone-2 mutations are in
  # docs/design/specs/web/gui-mutations.feature (slice W3).

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And the admin minted leader "lead2"
    And leader "lead" created space "demo"
    And leader "lead2" joined space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" created space "other"

  Rule: Listing spaces

    Scenario: The operator lists every space with its counts
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 2       |        | 1       |
        | other | 1       |        | 0       |

    Scenario: The operator sees a space it has no participant in
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the page lists space "demo"
      And the page lists space "other"

    Scenario: A joined admin is counted separately from the leaders
      Given the admin minted admin "ops"
      And admin "ops" joined space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 2       | ops    | 1       |
        | other | 1       |        | 0       |

    Scenario: An admin that did not join is not counted in a space
      Given the admin minted admin "ops"
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 2       |        | 1       |
        | other | 1       |        | 0       |

    Scenario: A leader session lists every space with its counts
      Given the operator signed in with the token of leader "lead"
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 2       |        | 1       |
        | other | 1       |        | 0       |

    Scenario: A member session lists only its own space
      Given the operator signed in with the token of member "worker"
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 2       |        | 1       |

    Scenario: A space with no members shows zero members
      Given leader "lead2" left space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      Then the page lists:
        | space | leaders | admins | members |
        | demo  | 1       |        | 1       |
        | other | 1       |        | 0       |

  Rule: Showing the participants of a space

    Scenario: The participants of a space are listed admins first, then leaders, then members
      Given the admin minted leader "amy"
      And leader "amy" joined space "demo"
      And leader "lead" invited member "bob" to space "demo"
      And leader "lead" invited member "abe" to space "demo"
      Given the admin minted admin "ops"
      And admin "ops" joined space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists the participants:
        | name   | role   |
        | ops    | admin  |
        | amy    | leader |
        | lead   | leader |
        | lead2  | leader |
        | abe    | member |
        | bob    | member |
        | worker | member |

    Scenario: An admin that did not join is not a participant
      Given the admin minted admin "ops"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page does not list "ops" as a participant
      And the page shows the web GUI is signed in as admin "admin"

    Scenario: The signed-in admin is named, not a placeholder
      Given the admin minted admin "ops"
      And the operator signed in with the token of admin "ops"
      When the operator opens the space "demo"
      Then the page shows the web GUI is signed in as admin "ops"

    Scenario: The participant count on the space list matches the participant page
      Given the operator signed in with the instance admin token
      When the operator opens the space list
      And the operator opens the space "demo"
      Then the number of participants on the space page is the member count plus the leader count plus the admin count on the space list

    Scenario: The operator opens a space it is not a participant of
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the response status is 200
      And the page shows the participants, the topics and the blackboards of space "demo"

    Scenario: A member session sees the detail of its own space
      Given the operator signed in with the token of member "worker"
      When the operator opens the space "demo"
      Then the response status is 200

    Scenario: A leader session sees no detail of a space it does not participate in
      Given the admin minted leader "stranger"
      Given the operator signed in with the token of leader "stranger"
      When the operator opens the space "demo"
      Then the response status is 403
      And the page shows no participant, topic or blackboard of space "demo"

    Scenario: An unknown space is not found
      Given the operator signed in with the instance admin token
      When the operator opens the space "ghost"
      Then the response status is 404
      And the page says the space does not exist
