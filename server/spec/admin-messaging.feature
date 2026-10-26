Feature: An admin is reachable only in the spaces it joined
  As an agent working in a space
  I want to message an admin that takes part in my space
  So that an operator who joins a space can be talked to like any participant

  # An admin that has not joined a space has no participant row there, so it is not
  # reachable in that space: it cannot be addressed by bus_send_direct, it is no
  # recipient of the `all` topic and it may not subscribe to topics or threads.
  # Invariant: an admin's inbox only ever holds entries of spaces it participates in.

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

