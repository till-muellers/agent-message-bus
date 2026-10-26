Feature: Topics in the web GUI
  As a human operator of a bus
  I want to see which topics a space has open
  So that I know where the agents of a space are talking

  # Moved from docs/design/specs/web/topics.feature (slice W1b).
  #
  # The web GUI shows the same topic list the agents see: the open topics of a
  # space, the mandatory "all" topic first and then the other topics by name.
  # Topic messages live only in the inboxes of the subscribers; the web GUI
  # does not read or copy the inbox contents of any other agent (it shows only the
  # signed-in admin's own inbox, gui-inbox.feature).

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"

  Rule: Listing the topics of a space

    Scenario: The all topic comes first, then the other topics by name
      Given leader "lead" created topic "zeta" in space "demo"
      And leader "lead" created topic "alpha" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists the topics:
        | topic | createdBy |
        | all   | lead      |
        | alpha | lead      |
        | zeta  | lead      |

    Scenario: A space with no extra topic shows only the all topic
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists the topics:
        | topic | createdBy |
        | all   | lead      |

    Scenario: The web GUI shows who created a topic
      Given member "worker" created topic "notes" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page shows topic "notes" created by "worker"

    Scenario: A closed topic is not listed
      Given leader "lead" created topic "temp" in space "demo"
      And leader "lead" closed topic "temp" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page does not list topic "temp"

  Rule: The web GUI does not copy inbox contents

    # A topic message is delivered to the inboxes of the subscribers and is
    # gone when it is acknowledged. The web GUI never lists, copies or acks the
    # inbox entries of any other agent.

    Scenario: A posted topic message does not appear on the space page
      Given leader "lead" created topic "notes" in space "demo"
      And member "worker" posted to topic "notes" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page does not show the message body of the topic message

    Scenario: The web GUI leaves the inbox of every agent untouched
      Given leader "lead" created topic "notes" in space "demo"
      And member "worker" posted to topic "notes" in space "demo"
      When the operator opens the space "demo"
      Then member "lead" can still read its inbox entry about the topic message
