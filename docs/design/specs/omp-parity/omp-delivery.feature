Feature: omp delivery of inbox entries, aligned with omp's native peer messages
  As an omp agent connected to the bus
  I want bus mail to enter my session the way omp's own peer messages do
  So that a bus message from a remote agent feels like a message from a local subagent or peer

  # Target behaviour for extension/lib/deliver.ts, extension/lib/inbox.ts and
  # extension/lib/connection.ts. Native reference (omp 18.12.6):
  #   - a peer message to a busy agent is injected at the next step boundary,
  #     without interrupting the running tool batch (session/irc-bridge.ts:230-245);
  #   - a message from the parent to a busy child steers it (irc-bridge.ts:231-241);
  #   - an idle recipient is woken with a turn (irc-bridge.ts:258);
  #   - plan mode folds the message into context without a turn (irc-bridge.ts:247-257).
  # Extension equivalent: pi.sendMessage(..., { deliverAs: "aside" }) uses the same
  # step-boundary queue as native peer messages (irc-bridge.ts:113-117,
  # agent-session.ts:9686-9694, 9728-9765). Critical stays "steer".
  #
  # Delivery is driven by the entry's delivery class, set by the server
  # (docs/design/specs/mail/delivery-class.feature), never by its type:
  #   interrupt (every critical entry; critical is for leaders and admins) -> steer,
  #             the role of omp's parent steer;
  #   queue     (non-critical messages and posts) -> aside, the role of a peer message;
  #   deferred  (lifecycle notices) -> nextTurn (omp-blackboards.feature).
  # omp is the only supported host (PO-6): there is no fallback for hosts without aside.

  Background:
    Given a bus with leader "lead" and member "worker" in space "alpha"
    And member "worker" runs in an omp session with the extension loaded and agent credentials
    And the session holds the inbox stream

  Rule: Entries of class queue go straight into the input queue as asides

    Scenario: A direct message reaches a busy agent at the next step boundary
      Given member "worker" is in the middle of a run with a batch of tool calls in flight
      When leader "lead" sends a non-critical direct message "check the schema" to "worker"
      Then the running tool batch is not interrupted
      And the message is injected into the run at the next step boundary
      And the agent does not have to finish its whole run before it sees the message

    Scenario: A topic message reaches a busy agent at the next step boundary
      Given member "worker" is subscribed to topic "dev"
      And member "worker" is in the middle of a run
      When leader "lead" sends a non-critical message to topic "dev"
      Then the message is injected into the run at the next step boundary

    Scenario: A non-critical entry ends a running interruptible wait
      Given member "worker" is blocked in omp's "wait" tool on a background job
      When leader "lead" sends a non-critical direct message to "worker"
      Then the wait returns early
      And the message is injected before the agent continues

    Scenario: Several entries arriving together keep their inbox order
      Given member "worker" is in the middle of a run
      When entries "e1", "e2" and "e3" arrive in this order
      Then they are injected in the order "e1, e2, e3"

  Rule: Entries of class interrupt steer, like a parent's message to its subagent

    Scenario: A critical direct message steers a busy agent
      Given member "worker" is in the middle of a run
      When leader "lead" sends a critical direct message "stop, wrong branch" to "worker"
      Then the message is delivered as a steer
      And the agent sees it before its next model call, ahead of any queued aside

    Scenario: A critical entry is marked as critical in the session message
      When leader "lead" sends a critical direct message to "worker"
      Then the delivered session message is marked critical

  Rule: An idle agent is woken, as omp wakes an idle peer

    Scenario Outline: An entry wakes an idle session in <mode> mode
      Given the session runs in "<mode>" mode
      And member "worker" is idle
      When leader "lead" sends a <kind> direct message to "worker"
      Then a turn starts with the message in context

      Examples:
        | mode | kind         |
        | tui  | non-critical |
        | tui  | critical     |
        | rpc  | non-critical |
        | rpc  | critical     |

    Scenario: Plan mode folds the entry into context without a turn
      Given the session is in plan mode
      And member "worker" is idle
      When leader "lead" sends a non-critical direct message to "worker"
      Then the message is added to the context and shown in the transcript
      And no turn starts

    Scenario: After the user interrupted the run, an entry does not restart it
      # omp folds extension asides into context while a user interrupt is in effect
      # (agent-session.ts:1414-1428). Native peer messages still wake; see the
      # open question in analysis.md.
      Given the user interrupted the run with Esc and has not prompted since
      When leader "lead" sends a non-critical direct message to "worker"
      Then the message is added to the context and shown in the transcript
      And no turn starts until the user prompts again

  Rule: One-shot modes never start an extra turn

    # print/json run their own prompt right after session_start; an agent-initiated
    # turn would race it (extension/lib/deliver.ts:11-16). An aside to an idle
    # session starts a turn regardless of triggerTurn (extensions/types.ts:1857-1859),
    # so one-shot modes must not use it while idle. Instead the entry is queued with
    # deliverAs "nextTurn": it becomes part of the next model request of the session,
    # if one comes (for a worker that just started: the one-shot run's first request).

    Scenario Outline: In <mode> mode an entry for a busy session is still an aside
      Given the session runs in "<mode>" mode
      And member "worker" is in the middle of its one-shot run
      When leader "lead" sends a non-critical direct message to "worker"
      Then the message is injected into the run at the next step boundary

      Examples:
        | mode  |
        | print |
        | json  |

    Scenario Outline: In <mode> mode an entry for an idle session waits for the next turn
      Given the session runs in "<mode>" mode
      And member "worker" is idle
      When leader "lead" sends a non-critical direct message to "worker"
      Then no turn starts
      And the entry is queued for the next turn
      And the entry is not acknowledged until a model request contains it

      Examples:
        | mode  |
        | print |
        | json  |

    Scenario: An entry queued before the one-shot run starts is part of its first request
      Given the session runs in "print" mode and its run has not started yet
      And an entry was queued for the next turn
      When the one-shot run makes its first model request
      Then the request contains the entry
      And the extension acknowledges it

  Rule: The ack watermark only moves over entries that reached the model

    # Today the loop acks right after pi.sendMessage returns (extension/lib/inbox.ts:69-77),
    # before the entry is in context; an entry queued in a session that then ends is lost.
    # bus_inbox_ack is cumulative (deletes every id <= upTo, server/spec/inbox.feature:112),
    # and a critical steer can be consumed before older queued asides. So the extension
    # acks a contiguous watermark: the highest id such that every lower delivered id has
    # been consumed. "Consumed" = part of a model request (aside, steer, nextTurn, or folded
    # into context in plan mode or after Esc), or returned by bus_wait.
    # A nextTurn entry (one-shot idle, lifecycle notices) holds the watermark until the next
    # model request; later entries stay unacknowledged until then and are replayed after a
    # crash. Accepted: duplicates are safer than loss, and any later turn releases it.
    # Contract owned by the mail BA: docs/design/specs/mail/omp-mail-delivery.feature.

    Scenario: An injected entry is acknowledged after it entered the context
      Given member "worker" is in the middle of a run
      When entry "e1" is injected at the next step boundary
      Then the extension acknowledges with bus_inbox_ack up to the id of "e1"

    Scenario: A steer consumed before older asides does not ack past them
      Given member "worker" is in the middle of a run
      And non-critical entries "e1" and "e2" are queued as asides
      When critical entry "e3" arrives and is consumed as a steer before "e1" and "e2"
      Then the extension does not acknowledge "e3" yet
      When "e1" and "e2" are injected
      Then the extension acknowledges up to the id of "e3"

    Scenario: An entry returned by bus_wait counts as consumed
      Given member "worker" called bus_wait
      When entry "e1" arrives and bus_wait returns it
      Then the extension acknowledges up to the id of "e1"

    Scenario: An entry still queued when the session ends is replayed later
      Given member "worker" is in the middle of a run
      And an entry is queued but the run ends before the next step boundary
      When the session shuts down
      Then the entry is not acknowledged
      And the next session with the same identity receives it again from the stream backlog

    Scenario: An entry dropped by a session switch is replayed in the new session
      Given an entry is queued as an aside
      When the user switches to another session before it is injected
      Then the entry is not acknowledged
      And the reconnected stream delivers it again

  Rule: Subagents stay out of the inbox

    # Subagents get only the blackboard read/post tools (omp-subagents.feature).

    Scenario: A subagent session neither streams nor receives bus entries
      Given member "worker" spawned an omp subagent
      When leader "lead" sends a direct message to "worker"
      Then only the main session of "worker" receives the entry
      And the main session forwards nothing to the subagent on its own
