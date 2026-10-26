Feature: Blackboards in omp
  As an omp agent and as the user watching it
  I want blackboard activity to reach me as short notices, with the thread one call away
  So that blackboards hold the shared state while my context only gets what changed

  # omp has no native blackboard. Its closest relatives are `local://` shared
  # artifacts and `read agent://<id>` outputs: documents an agent reads on demand.
  # Today every thread post arrives as a full message (body up to 64 KiB) through the
  # same path as direct messages (extension/lib/format.ts:12-17).
  # Overlaps with docs/design/specs/mail/ ("you have mail" notifications).

  Background:
    Given a bus with leader "lead" and members "worker" and "peer" in space "alpha"
    And member "peer" created blackboard "plan" with thread 7 "design"
    And member "worker" subscribed to thread 7
    And member "worker" runs in an omp TUI session with the extension loaded

  Rule: A thread post arrives as a short notice

    Scenario: A short post is shown whole
      When "peer" adds a post of 3 lines to thread 7
      Then member "worker" gets one notice naming blackboard "plan", thread 7 "design" and author "peer"
      And the notice contains the whole post

    Scenario: A long post is cut in the notice
      When "peer" adds a post of 200 lines to thread 7
      Then the notice contains the first lines of the post and how many lines were left out
      And the notice says to read the full thread with bus_read_thread for thread 7

    Scenario: The notice is delivered like any non-critical entry
      Given member "worker" is in the middle of a run
      When "peer" adds a post to thread 7
      Then the notice is injected at the next step boundary

    Scenario: A critical post still steers
      When leader "lead" adds a critical post to thread 7
      Then the notice is delivered as a steer and contains the whole post

  Rule: Several posts to one thread become one notice

    Scenario: Posts that arrive before the notice is injected are merged
      Given member "worker" is in the middle of a long tool call
      When "peer" adds 3 posts to thread 7 before the next step boundary
      Then member "worker" gets one notice for thread 7 listing the 3 new posts
      And all 3 entries are acknowledged once that notice entered the context

    Scenario: Posts to different threads stay separate
      Given member "worker" subscribed to thread 8 "tests" on blackboard "plan"
      When "peer" adds a post to thread 7 and a post to thread 8
      Then member "worker" gets one notice for thread 7 and one for thread 8

  Rule: Entries of class deferred do not wake an idle agent

    # Decided (PO-5, round 6): lifecycle notices are delivered on the next turn without
    # waking the agent. The server gives them class "deferred"
    # (docs/design/specs/mail/delivery-class.feature); the extension maps the class, not the type,
    # to nextTurn. They count as consumed when the next model request contains them, so
    # they hold the ack watermark until then (omp-delivery.feature, ack rule).

    Scenario: A deferred entry for an idle agent waits for its next turn
      Given member "worker" is idle
      When an entry of class "deferred" arrives
      Then no turn starts
      And the notice is part of the context of the agent's next turn

    Scenario: Lifecycle notices arrive as deferred entries
      When leader "lead" removes thread 7
      Then member "worker" receives a "thread_removed" entry with class "deferred"

  Rule: The user sees blackboard activity in the status line

    Scenario: Unread posts are counted per thread
      Given the user has not looked at thread 7
      When "peer" adds 2 posts to thread 7
      Then the status line shows "bus: 2 new posts in plan/7"

    Scenario: Reading the thread clears its count
      Given the status line shows new posts in thread 7
      When member "worker" calls bus_read_thread for thread 7
      Then thread 7 no longer counts in the status line

    Scenario: The status line is cleared on logout
      When the user runs "/bus logout"
      Then the status line has no bus entry

  Rule: A thread read renders as a document

    Scenario: bus_read_thread renders posts as a readable thread
      When member "worker" calls bus_read_thread for thread 7
      Then the transcript shows the thread title, then each post with author and time
      And long threads are collapsed to their newest posts until expanded
