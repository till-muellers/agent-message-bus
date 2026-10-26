Feature: Topics
  As a participant of a space
  I want to open, subscribe to, publish on and close named topics
  So that a message reaches exactly the agents interested in a subject

  # Tools: bus_list_topics, bus_create_topic, bus_send_topic,
  # bus_subscribe_topic, bus_unsubscribe_topic, bus_close_topic.
  # Every space has the built-in topic "all", opened by the space creator together
  # with the space. It reaches every participant and is never subscribed, closed
  # or recreated. Only open topics are visible; a closed topic behaves as unknown.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "ann" to space "demo"
    And leader "lead" invited member "bob" to space "demo"

  Rule: Listing topics

    Scenario: Open topics are listed with "all" first, then by name
      Given member "ann" created topic "zeta" in space "demo"
      And member "bob" created topic "beta" in space "demo"
      And member "ann" created topic "gone" in space "demo"
      And member "ann" closed topic "gone" in space "demo"
      When member "ann" lists the topics of space "demo"
      Then the call returns these topics in order:
        | name | createdBy | subscribed |
        | all  | lead      | true       |
        | beta | bob       | false      |
        | zeta | ann       | true       |

    Scenario: A fresh space lists only the "all" topic, always subscribed
      When member "bob" lists the topics of space "demo"
      Then the call returns these topics in order:
        | name | createdBy | subscribed |
        | all  | lead      | true       |

    Scenario: The subscribed flag reflects the caller's own subscriptions
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      When leader "lead" lists the topics of space "demo"
      Then the call returns these topics in order:
        | name | createdBy | subscribed |
        | all  | lead      | true       |
        | dev  | ann       | false      |

    Scenario: Listing topics has no side effects
      When member "ann" lists the topics of space "demo"
      Then "ann" receives no inbox entry
      And "bob" receives no inbox entry
      And "lead" receives no inbox entry

  Rule: Creating topics

    Scenario: Creating a topic subscribes its creator
      When member "ann" creates topic "dev" in space "demo"
      Then the call succeeds
      And topic "dev" in space "demo" is listed with createdBy "ann"
      And member "ann" is subscribed to topic "dev" in space "demo"

    Scenario Outline: Topic names must follow the name rule
      When member "ann" creates topic "<name>" in space "demo"
      Then the call fails with invalid "topic must match ^[a-z0-9][a-z0-9_-]{0,62}$"

      Examples:
        | name     |
        | Bad Name |
        | -dev     |
        | _dev     |
        | Dev      |
        |          |

    Scenario: Topic names are validated before the space is resolved
      When member "ann" creates topic "Bad Name" in space "nope"
      Then the call fails with invalid "topic must match ^[a-z0-9][a-z0-9_-]{0,62}$"

    Scenario: The "all" topic cannot be created
      When member "ann" creates topic "all" in space "demo"
      Then the call fails with conflict "topic all is already open"

    Scenario: An open topic cannot be created twice
      Given member "ann" created topic "dev" in space "demo"
      When member "bob" creates topic "dev" in space "demo"
      Then the call fails with conflict "topic dev is already open"

    Scenario: A closed topic's name can be reused
      Given member "ann" created topic "dev" in space "demo"
      And leader "lead" closed topic "dev" in space "demo"
      When member "bob" creates topic "dev" in space "demo"
      Then the call succeeds
      And topic "dev" in space "demo" is listed with createdBy "bob"

  Rule: Sending to a topic

    Scenario: A topic message reaches every subscriber except the sender
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And leader "lead" subscribed to topic "dev" in space "demo"
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then the call returns {"ok": true, "receipts": [{"to": "bob", "delivery": "queued"}, {"to": "lead", "delivery": "queued"}]}
      And member "bob" receives a "topic_message" inbox entry from "ann" with topic "dev" and body "m"
      And leader "lead" receives a "topic_message" inbox entry from "ann" with topic "dev" and body "m"
      And "ann" receives no inbox entry

    Scenario: Participants who are not subscribed do not receive topic messages
      Given member "ann" created topic "dev" in space "demo"
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then the call succeeds
      And "bob" receives no inbox entry
      And "lead" receives no inbox entry

    Scenario: Each recipient gets exactly one entry
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then member "bob" has exactly 1 inbox entry

    Scenario: A leader message to "all" reaches every participant except the sender
      When leader "lead" sends "hey" to topic "all" in space "demo"
      Then the call succeeds
      And member "ann" receives a "topic_message" inbox entry from "lead" with topic "all" and body "hey"
      And member "bob" receives a "topic_message" inbox entry from "lead" with topic "all" and body "hey"
      And "lead" receives no inbox entry

    Scenario: "all" reaches every participant without any subscription
      Given leader "lead2" joined space "demo"
      When leader "lead2" sends "hey" to topic "all" in space "demo"
      Then leader "lead" receives a "topic_message" inbox entry from "lead2" with topic "all"
      And member "ann" receives a "topic_message" inbox entry from "lead2" with topic "all"
      And member "bob" receives a "topic_message" inbox entry from "lead2" with topic "all"
      And "lead2" receives no inbox entry

    Scenario Outline: Only leaders may send to "all"
      When <role> "<sender>" sends "x" to topic "all" in space "demo"
      Then <outcome>

      Examples:
        | role   | sender | outcome                                                    |
        | leader | lead   | the call succeeds                                          |
        | member | ann    | the call fails with forbidden "only leaders may send to all" |

    Scenario: A rejected message to "all" is delivered to nobody
      When member "ann" sends "x" to topic "all" in space "demo"
      Then the call fails with forbidden "only leaders may send to all"
      And "bob" receives no inbox entry
      And "lead" receives no inbox entry

    Scenario Outline: Only leaders may send critical topic messages
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And leader "lead" subscribed to topic "dev" in space "demo"
      When <role> "<sender>" sends critical "x" to topic "<topic>" in space "demo"
      Then <outcome>

      Examples:
        | role   | sender | topic | outcome                                                              |
        | leader | lead   | dev   | the call succeeds                                                    |
        | leader | lead   | all   | the call succeeds                                                    |
        | member | ann    | dev   | the call fails with forbidden "only leaders may send critical messages" |
        | member | ann    | all   | the call fails with forbidden "only leaders may send critical messages" |

    Scenario: A critical topic message is flagged critical for each recipient
      Given member "ann" created topic "dev" in space "demo"
      And leader "lead" subscribed to topic "dev" in space "demo"
      When leader "lead" sends critical "now" to topic "dev" in space "demo"
      Then member "ann" receives a "topic_message" inbox entry from "lead" with critical true, topic "dev" and body "now"

    Scenario Outline: The body must be between 1 and 65536 bytes
      Given member "ann" created topic "dev" in space "demo"
      When member "ann" sends a body of <size> bytes to topic "dev" in space "demo"
      Then <outcome>

      Examples:
        | size  | outcome                                                   |
        | 0     | the call fails with invalid "body must be 1..65536 bytes" |
        | 65536 | the call succeeds                                         |
        | 65537 | the call fails with invalid "body must be 1..65536 bytes" |

    Scenario: Body and critical checks run before the topic lookup
      When member "ann" sends a body of 0 bytes to topic "nope" in space "demo"
      Then the call fails with invalid "body must be 1..65536 bytes"
      When member "ann" sends critical "x" to topic "nope" in space "demo"
      Then the call fails with forbidden "only leaders may send critical messages"

    Scenario: An unknown topic cannot be sent to
      When member "ann" sends "x" to topic "nope" in space "demo"
      Then the call fails with not_found "topic nope not found in space demo"

    Scenario: A closed topic cannot be sent to
      Given member "ann" created topic "dev" in space "demo"
      And member "ann" closed topic "dev" in space "demo"
      When member "ann" sends "x" to topic "dev" in space "demo"
      Then the call fails with not_found "topic dev not found in space demo"

  Rule: Subscribing and unsubscribing

    Scenario: Subscribing to a topic starts delivery
      Given member "ann" created topic "dev" in space "demo"
      When member "bob" subscribes to topic "dev" in space "demo"
      Then the call succeeds
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then member "bob" receives a "topic_message" inbox entry from "ann" with topic "dev" and body "m"

    Scenario: Subscribing is idempotent
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      When member "bob" subscribes to topic "dev" in space "demo"
      Then the call succeeds

    Scenario: Unsubscribing stops delivery
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And member "bob" unsubscribed from topic "dev" in space "demo"
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then "bob" receives no inbox entry

    Scenario: Unsubscribing is idempotent
      Given member "ann" created topic "dev" in space "demo"
      When member "bob" unsubscribes from topic "dev" in space "demo"
      Then the call succeeds

    Scenario: The creator may unsubscribe from their own topic
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      When member "ann" unsubscribes from topic "dev" in space "demo"
      Then the call succeeds
      When member "bob" sends "m" to topic "dev" in space "demo"
      Then "ann" receives no inbox entry

    Scenario Outline: The "all" topic cannot be subscribed or unsubscribed
      When member "ann" <action> topic "all" in space "demo"
      Then the call fails with invalid "the all topic cannot be subscribed or unsubscribed"

      Examples:
        | action             |
        | subscribes to      |
        | unsubscribes from  |

    Scenario: The "all" check runs before the space is resolved
      When member "ann" subscribes to topic "all" in space "nope"
      Then the call fails with invalid "the all topic cannot be subscribed or unsubscribed"

    Scenario Outline: Unknown or closed topics cannot be subscribed or unsubscribed
      Given member "ann" created topic "dev" in space "demo"
      And member "ann" closed topic "dev" in space "demo"
      When member "bob" <action> topic "<topic>" in space "demo"
      Then the call fails with not_found "topic <topic> not found in space demo"

      Examples:
        | action            | topic |
        | subscribes to     | nope  |
        | unsubscribes from | nope  |
        | subscribes to     | dev   |
        | unsubscribes from | dev   |

    Scenario: Leaving a space drops the leaver's topic subscriptions
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And member "bob" left space "demo"
      When member "ann" sends "m" to topic "dev" in space "demo"
      Then "bob" receives no inbox entry

  Rule: Closing topics

    Scenario: Subscribers except the closer are notified when a topic closes
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      And leader "lead" subscribed to topic "dev" in space "demo"
      When leader "lead" closes topic "dev" in space "demo"
      Then the call succeeds
      And member "ann" receives a "topic_closed" inbox entry from "lead" with topic "dev" and no body
      And member "bob" receives a "topic_closed" inbox entry from "lead" with topic "dev" and no body
      And "lead" receives no inbox entry

    Scenario: Participants not subscribed at close time are not notified
      Given member "ann" created topic "dev" in space "demo"
      And member "ann" unsubscribed from topic "dev" in space "demo"
      When leader "lead" closes topic "dev" in space "demo"
      Then the call succeeds
      And "ann" receives no inbox entry
      And "bob" receives no inbox entry

    Scenario Outline: Only the creator or a leader may close a topic
      Given leader "lead2" joined space "demo"
      And member "ann" created topic "dev" in space "demo"
      When <role> "<closer>" closes topic "dev" in space "demo"
      Then <outcome>

      Examples:
        | role   | closer | outcome                                                                         |
        | member | ann    | the call succeeds                                                               |
        | leader | lead   | the call succeeds                                                               |
        | leader | lead2  | the call succeeds                                                               |
        | member | bob    | the call fails with forbidden "only the creator or a leader may close topic dev" |

    Scenario: A rejected close leaves the topic open
      Given member "ann" created topic "dev" in space "demo"
      When member "bob" closes topic "dev" in space "demo"
      Then the call fails with forbidden "only the creator or a leader may close topic dev"
      And topic "dev" in space "demo" is listed with createdBy "ann"
      And "ann" receives no inbox entry

    Scenario: Closing drops every subscription and unlists the topic
      Given member "ann" created topic "dev" in space "demo"
      And member "bob" subscribed to topic "dev" in space "demo"
      When member "ann" closes topic "dev" in space "demo"
      Then the call succeeds
      And topic "dev" is not listed in space "demo"
      When member "bob" creates topic "dev" in space "demo"
      And member "bob" sends "m" to topic "dev" in space "demo"
      Then "ann" receives no inbox entry

    Scenario: The "all" topic cannot be closed
      When leader "lead" closes topic "all" in space "demo"
      Then the call fails with forbidden "the all topic cannot be closed"

    Scenario: The "all" check runs before the space is resolved
      When leader "lead" closes topic "all" in space "nope"
      Then the call fails with forbidden "the all topic cannot be closed"

    Scenario Outline: Unknown or already closed topics cannot be closed
      Given member "ann" created topic "dev" in space "demo"
      And member "ann" closed topic "dev" in space "demo"
      When leader "lead" closes topic "<topic>" in space "demo"
      Then the call fails with not_found "topic <topic> not found in space demo"

      Examples:
        | topic |
        | nope  |
        | dev   |

    Scenario: The topic lookup runs before the permission check
      When member "bob" closes topic "nope" in space "demo"
      Then the call fails with not_found "topic nope not found in space demo"

  Rule: Every topic operation is gated by space membership

    Scenario Outline: An admin that has not joined cannot subscribe (<action>)
      Given member "ann" created topic "dev" in space "demo"
      When the admin <action> in space "demo"
      Then the call fails with forbidden "join space demo first; only participants receive messages"

      Examples:
        | action                        |
        | subscribes to topic "dev"     |
        | unsubscribes from topic "dev" |

    Scenario Outline: Topic operations require an existing space
      When leader "lead" <action> in space "nope"
      Then the call fails with not_found "space nope not found"

      Examples:
        | action                        |
        | lists the topics              |
        | creates topic "dev"           |
        | sends "x" to topic "all"      |
        | subscribes to topic "dev"     |
        | unsubscribes from topic "dev" |
        | closes topic "dev"            |

    Scenario Outline: A leader who is not a participant cannot use topics
      Given the admin minted leader "lead2"
      When leader "lead2" <action> in space "demo"
      Then the call fails with forbidden "not a participant of space demo"

      Examples:
        | action                        |
        | lists the topics              |
        | creates topic "dev"           |
        | sends "x" to topic "all"      |
        | subscribes to topic "dev"     |
        | unsubscribes from topic "dev" |
        | closes topic "dev"            |
