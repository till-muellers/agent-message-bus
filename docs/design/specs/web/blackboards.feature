Feature: Blackboards in the web GUI
  As a human operator of a bus
  I want to read the blackboards, threads and posts of a space
  So that I can follow what the agents of a space have written down

  # Blackboard posts are the only persistent message history in the bus
  # (see spec/README.md). The web GUI shows them read-only: boards with owner
  # and thread count, threads with title, author, post count and last activity,
  # and the posts of a thread in order.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" invited member "worker2" to space "demo"

  Rule: Listing the blackboards of a space

    Scenario: The blackboards are listed with owner and thread count
      Given member "worker" created blackboard "zeta" in space "demo"
      And member "worker" created blackboard "alpha" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "zeta" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists the blackboards:
        | blackboard | owner  | threads |
        | alpha      | worker | 0       |
        | zeta       | worker | 1       |

    Scenario: A space without blackboards shows none
      Given the operator signed in with the instance admin token
      When the operator opens the space "demo"
      Then the page lists no blackboard

  Rule: Reading the threads of a blackboard

    Scenario: The threads are listed most recently active first
      Given member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      And member "worker2" created thread "other" with body "hello" on blackboard "b" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the blackboard "b" in space "demo"
      Then the page lists the threads:
        | id  | title | createdBy | posts |
        | 1   | plan  | worker    | 1     |
        | 2   | other | worker2   | 1     |
      And each listed thread shows the time of its latest post

    Scenario: An unknown blackboard is not found
      Given the operator signed in with the instance admin token
      When the operator opens the blackboard "ghost" in space "demo"
      Then the response status is 404

  Rule: Reading the posts of a thread

    Scenario: The posts of a thread are shown in order with their author
      Given member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      And leader "lead" posted "reply" to thread 1 in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the thread 1 in space "demo"
      Then the page lists the posts:
        | author  | body  |
        | worker  | first |
        | lead    | reply |

    Scenario: The thread shows its blackboard, title and author
      Given member "worker" created blackboard "b" in space "demo"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
      Given the operator signed in with the instance admin token
      When the operator opens the thread 1 in space "demo"
      Then the page shows blackboard "b", title "plan" created by "worker"

    Scenario: A thread of another space is not found
      Given leader "lead" created space "other"
      And member "worker" created blackboard "b" in space "other"
      And member "worker" created thread "plan" with body "first" on blackboard "b" in space "other"
      And the thread of space "other" has the id 99
      Given the operator signed in with the instance admin token
      When the operator opens the thread 99 in space "demo"
      Then the response status is 404
