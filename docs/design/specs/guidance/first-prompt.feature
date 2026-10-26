Feature: The invite task arrives as a direct message from the inviting leader
  As a member agent that was spawned or invited by a leader
  I want my first prompt to arrive as a direct message from that leader, like every other bus entry
  So that I know it is an order from my leader, whom to report to, and that it came through the bus

  # TODO "Make the first prompt recognisable as coming from the bus". PO ruling (round 3): option A.
  # Today: the invite stores `task`; POST /v1/invites/accept returns it once and the omp
  # extension sends it with pi.sendUserMessage (extension/index.ts:103) as a plain user message.
  # Decided: the invite also stores the inviter's name. Accepting the code stores the task as a
  # `direct_message` inbox entry from the inviter to the new member, in the same transaction as
  # the admission (invites runs it through inbox.Deliver), and publishes it after commit.
  # `task` is removed from the accept response. The omp extension no longer sends a user message;
  # the entry reaches the session through the inbox stream like every other entry: an
  # `agent-bus` direct_message entry from the inviter with the task as its body. Its envelope
  # text (header, fence, reply hint) is owned by docs/design/specs/omp-parity/omp-rendering.feature;
  # the steps below only assert type, sender and body.
  # Delivery policy (owned by the omp-parity BA, docs/design/specs/omp-parity/omp-delivery.feature):
  # normal entries are delivered as `aside` (an aside to an idle tui/rpc session starts a turn),
  # critical ones as `steer`; in print/json mode an entry for an idle session is queued for the
  # next turn (`nextTurn`) without starting one; an entry is acknowledged once it is part of a
  # model request.
  # Server/extension skew: a new extension on an old server would lose the task (the old server
  # returns it in the accept response, the new extension ignores it). The extension refuses a
  # server whose protocol is too old (docs/design/specs/findings/plugin-version-skew.feature).
  # The task is never critical.
  # Admins are named agents (PO ruling, round 4; docs/design/specs/admin/). An admin invites under its own
  # name. An admin that has not joined the space cannot be messaged there: the worker's report
  # fails with not_found "participant ops not in space demo", and the guide tells it to report to
  # a leader of the space instead (guide-draft.md "Members").

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"

  Rule: The invite records who invited (server)

    Scenario: An invite code remembers the inviting leader
      When leader "lead" invites member "worker" to space "demo" with task "fix the bug"
      Then the stored invite records "lead" as the inviter

    Scenario: A second leader's invite records that leader
      Given the admin minted leader "lead2"
      And leader "lead2" joined space "demo"
      When leader "lead2" invites member "worker" to space "demo" with task "write the docs"
      Then the stored invite records "lead2" as the inviter

    Scenario: An invite by an admin records the admin's name
      Given the admin minted admin "ops"
      When admin "ops" invites member "worker" to space "demo" with task "audit the board"
      Then the stored invite records "ops" as the inviter

  Rule: Accepting an invite with a task stores it as a direct message (server)

    Scenario: The task becomes the member's first inbox entry
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then the status is 200 and the body is {"name":"worker","space":"demo","token":"amb_…"}
      And the body has no "task" field
      And member "worker" has exactly one inbox entry in space "demo"
      And that entry has type "direct_message", from "lead", critical false and body "fix the bug"

    Scenario: The task entry is published after the admission commits
      Given a listener on the live channel of "worker"
      And leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the code is posted to /v1/invites/accept
      Then the listener receives exactly one wake-up signal after the transaction commits

    Scenario: An invite without a task delivers nothing
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When the code is posted to /v1/invites/accept
      Then member "worker" has no inbox entries

    Scenario: A failed admission delivers nothing and keeps the code
      Given leader "lead" created an invite code for member "x" in space "demo" with task "fix the bug"
      And the admin minted leader "x"
      When the code is posted to /v1/invites/accept
      Then the call fails with conflict "agent x is a leader agent"
      And no inbox entry exists for "x"
      And the invite code is still stored

    Scenario: The task is sent from the inviter even if the inviter has left the space since
      Given the admin minted leader "lead2"
      And leader "lead2" joined space "demo"
      And leader "lead2" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And leader "lead2" left space "demo"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "lead2" with body "fix the bug"

    Scenario: A task from an admin arrives from the admin's name
      Given the admin minted admin "ops"
      And admin "ops" created an invite code for member "worker" in space "demo" with task "audit the board"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "ops" with body "audit the board"

    Scenario: A task from the bootstrap admin arrives from "admin"
      Given the bootstrap admin created an invite code for member "worker" in space "demo" with task "audit the board"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "admin" with body "audit the board"

    Scenario: Re-inviting an existing member with a task sends another direct message
      Given leader "lead" invited member "worker" to space "demo"
      And leader "lead" created an invite code for member "worker" in space "demo" with task "second task"
      When the code is posted to /v1/invites/accept
      Then member "worker" has an inbox entry from "lead" with body "second task"

    Scenario: A client without the omp extension reads the task from its inbox
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And the code was accepted and returned a member token
      When member "worker" reads its inbox with that token
      Then the call returns one entry with type "direct_message", from "lead" and body "fix the bug"

  Rule: The omp worker receives the task like any other direct message (extension)

    Scenario: A spawned worker starts working on the task from its leader
      Given leader "lead" spawned worker "worker" in space "demo" with task "fix the bug"
      When the worker session starts in tui mode and redeems --bus-accept
      Then the session shows "Joined space demo as member worker."
      And no user message is sent by the extension
      And the session receives an agent-bus direct_message entry from "lead" with body "fix the bug" as an aside
      And that message starts a turn

    Scenario: /bus accept delivers the task the same way
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When the user runs "/bus accept <url> <code>" in an idle tui session
      Then the session receives an agent-bus direct_message entry from "lead" with body "fix the bug" as an aside
      And that message starts a turn

    Scenario: A busy session gets the task at the next step boundary
      Given an omp session in tui mode in the middle of a run with tool calls in flight
      When the user runs "/bus accept <url> <code>" for an invite with task "fix the bug"
      Then the task message is injected at the next step boundary without interrupting the run

    Scenario: The task survives a worker that cannot stream yet
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And another session already streams the inbox of "worker"
      When a new session accepts the code
      Then the task stays in the inbox of "worker" until a session streams it and it reaches that session's context

  Rule: A one-shot worker waits briefly for its task before its run starts (extension)

    # Closes the print-mode race (analysis.md section 5.4b, accepted risk R1). omp awaits the
    # session_start handlers before a print/json run starts (pi-coding-agent
    # src/modes/runtime-init.ts:212, src/modes/print-mode.ts:167,434). After redeeming
    # --bus-accept in print or json mode, the extension reads the inbox once with bus_inbox_read
    # (not acknowledging) and lets session_start finish only when the stream has queued every
    # entry up to the highest id it read for the run's first model request (`nextTurn`), or
    # after 5 seconds.
    # No server change: the task entry is committed before the accept response returns.

    Scenario: In print mode the task is part of the one-shot run's first request
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When "omp -p <prompt> --bus-url <url> --bus-accept <code>" starts
      Then session_start does not finish before the direct_message entry from "lead" with body "fix the bug" was queued for the next turn
      And it was queued without starting a turn
      And the one-shot run's first model request contains it
      And the extension then acknowledges it

    Scenario: The wait gives up after 5 seconds
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      And the inbox stream does not deliver within 5 seconds
      When "omp -p <prompt> --bus-url <url> --bus-accept <code>" starts
      Then session_start finishes after 5 seconds
      And the extension logs "agent-bus: inbox backlog not delivered within 5 s; continuing"
      And the task follows the normal print-mode delivery when it arrives

    Scenario: Nothing waiting, no wait
      Given leader "lead" created an invite code for member "worker" in space "demo"
      When "omp -p <prompt> --bus-url <url> --bus-accept <code>" starts
      Then session_start does not wait for the stream

    Scenario: Interactive sessions do not wait
      Given leader "lead" created an invite code for member "worker" in space "demo" with task "fix the bug"
      When "omp --bus-url <url> --bus-accept <code>" starts in tui mode
      Then session_start does not wait for the stream
      And the task starts a turn when it arrives

    Scenario: Only the invite redemption waits
      Given an omp print session that starts with stored member credentials and no --bus-accept
      When it starts
      Then session_start does not wait for the stream
      # A general "backlog before a one-shot run" rule would belong to the omp-parity BA.
