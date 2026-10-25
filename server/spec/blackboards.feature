Feature: Blackboards, threads and posts
  As a participant of a space
  I want shared blackboards with discussion threads I can subscribe to
  So that agents can collaborate asynchronously and get notified of new posts

  # Tools: bus_list_blackboards, bus_create_blackboard, bus_delete_blackboard,
  # bus_list_threads, bus_create_thread, bus_read_thread, bus_remove_thread,
  # bus_add_post, bus_subscribe_thread, bus_unsubscribe_thread.
  # Thread ids are database ids; in a fresh bus the first thread created gets id 1.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And leader "lead" invited member "worker2" to space "demo"
    And leader "lead" created space "other"
    And leader "lead" invited member "outsider" to space "other"

  # --- Space gate (shared resolver, applies to every blackboard operation) ---

  Scenario Outline: Only participants of the space may use its blackboards
    Given the admin minted leader "lead2"
    When <caller> lists the blackboards of space "<space>"
    Then the call fails with <code> "<message>"

    Examples:
      | caller            | space | code      | message                                                              |
      | the admin         | demo  | forbidden | admin token cannot act in spaces; mint a leader with bus_mint_leader |
      | leader "lead"     | ghost | not_found | space ghost not found                                                |
      | member "outsider" | demo  | forbidden | token is not valid for space demo                                    |
      | leader "lead2"    | demo  | forbidden | not a participant of space demo                                      |

  # --- bus_create_blackboard / bus_list_blackboards ---

  Scenario Outline: Any participant may create a blackboard and becomes its owner
    When <role> "<name>" creates blackboard "notes" in space "demo"
    Then the call succeeds
    And listing the blackboards of space "demo" returns
      | name  | owner  | threads |
      | notes | <name> | 0       |

    Examples:
      | role   | name   |
      | leader | lead   |
      | member | worker |

  Scenario: Blackboards are listed in name order with owner and thread count
    Given member "worker" created blackboard "zeta" in space "demo"
    And member "worker2" created blackboard "alpha" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "zeta" in space "demo"
    When leader "lead" lists the blackboards of space "demo"
    Then the call returns
      | name  | owner   | threads |
      | alpha | worker2 | 0       |
      | zeta  | worker  | 1       |

  Scenario: An empty space lists no blackboards
    When member "worker" lists the blackboards of space "demo"
    Then the call returns an empty list

  Scenario: Blackboard names must be valid
    When member "worker" creates blackboard "Bad Name" in space "demo"
    Then the call fails with invalid "blackboard must match ^[a-z0-9][a-z0-9_-]{0,62}$"

  Scenario: Blackboard names are unique within a space
    Given member "worker" created blackboard "b" in space "demo"
    When member "worker2" creates blackboard "b" in space "demo"
    Then the call fails with conflict "blackboard b already exists"

  Scenario: The same blackboard name may exist in different spaces
    Given member "worker" created blackboard "b" in space "demo"
    When member "outsider" creates blackboard "b" in space "other"
    Then the call succeeds
    And listing the blackboards of space "demo" returns
      | name | owner  | threads |
      | b    | worker | 0       |

  # --- bus_delete_blackboard ---

  Scenario Outline: Only the owner or a leader may delete a blackboard
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    And leader "lead" subscribed to thread 1 in space "demo"
    When <caller> deletes blackboard "b" in space "demo"
    Then <outcome>

    Examples:
      | caller             | outcome                                                                                   |
      | member "worker"    | the call succeeds                                                                         |
      | leader "lead"      | the call succeeds                                                                         |
      | member "worker2"   | the call fails with forbidden "only the owner or a leader may delete blackboard b"        |

  Scenario Outline: Deleting a blackboard notifies its thread subscribers except the caller
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    And leader "lead" subscribed to thread 1 in space "demo"
    When <role> "<caller>" deletes blackboard "b" in space "demo"
    Then the call succeeds
    And member "worker2" receives a "blackboard_removed" inbox entry from "<caller>" with blackboard "b"
    And <other> receives a "blackboard_removed" inbox entry from "<caller>" with blackboard "b"
    And "<caller>" receives no inbox entry
    And listing the blackboards of space "demo" returns an empty list

    Examples:
      | role   | caller | other                |
      | member | worker | leader "lead"        |
      | leader | lead   | member "worker"      |

  Scenario: A subscriber of several threads on a deleted board is notified once
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker" created thread "todo" with body "second" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    And member "worker2" subscribed to thread 2 in space "demo"
    When member "worker" deletes blackboard "b" in space "demo"
    Then member "worker2" receives exactly one "blackboard_removed" inbox entry from "worker" with blackboard "b"

  Scenario: Deleting a blackboard removes its threads and posts
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    When leader "lead" deletes blackboard "b" in space "demo"
    Then the call succeeds
    And member "worker" reading thread 1 in space "demo" fails with not_found "thread 1 not found"

  Scenario: Deleting a blackboard that does not exist
    When leader "lead" deletes blackboard "b" in space "demo"
    Then the call fails with not_found "blackboard b not found"

  Scenario: Deleting a blackboard twice
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" deleted blackboard "b" in space "demo"
    When leader "lead" deletes blackboard "b" in space "demo"
    Then the call fails with not_found "blackboard b not found"

  # --- bus_create_thread ---

  Scenario: Creating a thread adds its first post and subscribes the creator
    Given member "worker" created blackboard "b" in space "demo"
    When member "worker2" creates thread "plan" with body "first" on blackboard "b" in space "demo"
    Then the call returns threadId 1
    And member "worker2" reading thread 1 in space "demo" returns
      | blackboard | title | createdBy | subscribed |
      | b          | plan  | worker2   | true       |
    And thread 1 has posts
      | author  | body  |
      | worker2 | first |

  Scenario: Creating a thread on an unknown blackboard
    When member "worker" creates thread "plan" with body "first" on blackboard "nope" in space "demo"
    Then the call fails with not_found "blackboard nope not found"

  Scenario Outline: Thread title and first post body are validated
    Given member "worker" created blackboard "b" in space "demo"
    When member "worker" creates thread <title> with body <body> on blackboard "b" in space "demo"
    Then the call fails with invalid "<message>"

    Examples:
      | title                               | body                                  | message                          |
      | ""                                  | "x"                                   | title must be 1..200 characters  |
      | of 201 characters "é"               | "x"                                   | title must be 1..200 characters  |
      | "t"                                 | ""                                    | body must be 1..65536 bytes      |
      | "t"                                 | of 65537 bytes                        | body must be 1..65536 bytes      |

  Scenario: Title length is counted in characters, not bytes
    Given member "worker" created blackboard "b" in space "demo"
    When member "worker" creates thread of 200 characters "é" with body "x" on blackboard "b" in space "demo"
    Then the call succeeds

  # --- bus_list_threads ---

  Scenario: Threads are listed most recently active first
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" created thread "other" with body "hello" on blackboard "b" in space "demo"
    And leader "lead" posted "reply" to thread 1 in space "demo"
    When member "worker" lists the threads of blackboard "b" in space "demo"
    Then the call returns
      | id | title | createdBy | posts |
      | 1  | plan  | worker    | 2     |
      | 2  | other | worker2   | 1     |
    And each listed thread has lastPostAt set to the time of its latest post

  Scenario: Listing threads of an unknown blackboard
    When leader "lead" lists the threads of blackboard "nope" in space "demo"
    Then the call fails with not_found "blackboard nope not found"

  # --- bus_read_thread ---

  Scenario: Reading a thread returns its posts in order and the caller's subscription
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And leader "lead" posted "reply" to thread 1 in space "demo"
    When member "worker" reads thread 1 in space "demo"
    Then the call returns
      | blackboard | title | createdBy | subscribed |
      | b          | plan  | worker    | true       |
    And the returned posts are
      | author | body  |
      | worker | first |
      | lead   | reply |
    When leader "lead" reads thread 1 in space "demo"
    Then the returned subscribed flag is false

  # --- bus_add_post ---

  Scenario: Posting notifies subscribers except the author
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When member "worker2" posts "hi" to thread 1 in space "demo"
    Then the call returns a postId
    And member "worker" receives a "thread_post" inbox entry from "worker2" with blackboard "b", threadId 1, threadTitle "plan" and body "hi"
    And "worker2" receives no inbox entry
    And "lead" receives no inbox entry

  Scenario: Posting does not subscribe the author
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    When leader "lead" posts "reply" to thread 1 in space "demo"
    Then the call succeeds
    And leader "lead" reading thread 1 in space "demo" returns subscribed false

  Scenario Outline: Post bodies are validated
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    When member "worker" posts <body> to thread 1 in space "demo"
    Then the call fails with invalid "body must be 1..65536 bytes"

    Examples:
      | body           |
      | ""             |
      | of 65537 bytes |

  # --- bus_subscribe_thread / bus_unsubscribe_thread ---

  Scenario: Subscribing is idempotent
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When member "worker2" subscribes to thread 1 in space "demo"
    Then the call succeeds
    When member "worker" posts "hi" to thread 1 in space "demo"
    Then member "worker2" receives exactly one "thread_post" inbox entry from "worker" with body "hi"

  Scenario: Unsubscribing is idempotent and stops post notifications
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    And member "worker2" posted "hi" to thread 1 in space "demo"
    And member "worker" received a "thread_post" inbox entry from "worker2" with body "hi"
    When member "worker" unsubscribes from thread 1 in space "demo"
    Then the call succeeds
    When member "worker" unsubscribes from thread 1 in space "demo"
    Then the call succeeds
    When member "worker2" posts "again" to thread 1 in space "demo"
    Then member "worker" receives no further inbox entry

  # --- bus_remove_thread ---

  Scenario Outline: Only the creator or a leader may remove a thread
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When <caller> removes thread 1 in space "demo"
    Then <outcome>

    Examples:
      | caller             | outcome                                                                         |
      | member "worker"    | the call succeeds                                                               |
      | leader "lead"      | the call succeeds                                                               |
      | member "worker2"   | the call fails with forbidden "only the creator or a leader may remove thread 1" |

  Scenario Outline: Removing a thread deletes it and notifies subscribers except the caller
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When <role> "<caller>" removes thread 1 in space "demo"
    Then the call succeeds
    And member "worker2" receives a "thread_removed" inbox entry from "<caller>" with blackboard "b", threadId 1 and threadTitle "plan"
    And "<caller>" receives no inbox entry
    And member "worker" reading thread 1 in space "demo" fails with not_found "thread 1 not found"

    Examples:
      | role   | caller |
      | member | worker |
      | leader | lead   |

  Scenario: Removing a thread keeps its blackboard
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    When member "worker" removes thread 1 in space "demo"
    Then listing the blackboards of space "demo" returns
      | name | owner  | threads |
      | b    | worker | 0       |

  # --- Threads are scoped to their space ---

  Scenario Outline: A thread id from another space is not found
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    When <caller> <action> thread 1 in space "other"
    Then the call fails with not_found "thread 1 not found"

    Examples:
      | caller             | action                    |
      | leader "lead"      | reads                     |
      | member "outsider"  | posts "x" to              |
      | member "outsider"  | subscribes to             |
      | member "outsider"  | unsubscribes from         |
      | leader "lead"      | removes                   |

  # --- Departure (bus_leave_space) ---

  Scenario: A member leaving the space loses their blackboards
    Given member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When member "worker" leaves space "demo"
    Then the call succeeds
    And member "worker2" receives a "blackboard_removed" inbox entry from "worker" with blackboard "b"
    And listing the blackboards of space "demo" returns an empty list

  Scenario: A leader leaving the space keeps their blackboards
    Given the admin minted leader "lead2"
    And leader "lead2" joined space "demo"
    And leader "lead" created blackboard "b" in space "demo"
    And leader "lead" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And member "worker2" subscribed to thread 1 in space "demo"
    When leader "lead" leaves space "demo"
    Then the call succeeds
    And "worker2" receives no inbox entry
    And listing the blackboards of space "demo" returns
      | name | owner | threads |
      | b    | lead  | 1       |
    And leader "lead" has no thread subscriptions left in space "demo"
