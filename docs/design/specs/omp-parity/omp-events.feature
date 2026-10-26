Feature: Bus activity on the extension event bus (pi.events)
  As the author of another omp extension (a status widget, a notifier, a dashboard)
  I want to observe bus activity without opening my own inbox stream
  So that one session holds the stream and every extension can still react to it

  # pi.events is an in-process publish/subscribe bus shared by the extensions bound to
  # one session (utils/event-bus.ts:3-33; ExtensionAPI.events, extensions/types.ts:1964).
  # The main session's bus is created in main.ts:554/2377. A subagent session gets a new
  # one: task/executor.ts:4611-4678 passes no eventBus, so sdk.ts:1773 creates one.
  # pi.events therefore cannot carry anything between a main session and its subagents.
  # It does not persist, does not cross processes and does not put anything into the
  # model's context, so it is not a mailbox. Upstream pi documents it for
  # communication between extensions (pi-mono docs/extensions.md, "pi.events").
  # Channel names below are proposals.

  Background:
    Given member "worker" runs in an omp session with the extension loaded and agent credentials

  Rule: Every delivered entry is published

    Scenario: An entry is published after it was handed to the session
      Given another extension listens on channel "agent-bus:entry"
      When an entry arrives on the inbox stream
      Then the listener receives the entry with its delivery mode "aside", "steer" or "nextTurn"

    Scenario: A failing listener does not stop delivery
      Given another extension listens on channel "agent-bus:entry" and throws
      When an entry arrives on the inbox stream
      Then the entry is still delivered to the session and acknowledged

  Rule: Connection changes are published

    Scenario Outline: The connection state "<state>" is published
      Given another extension listens on channel "agent-bus:connection"
      When <event>
      Then the listener receives state "<state>" with the agent name and space

      Examples:
        | event                                          | state       |
        | the inbox stream opens                         | connected   |
        | another session already holds the stream       | conflict    |
        | the stream drops and the loop waits to retry   | retrying    |
        | the user runs "/bus logout"                    | logged-out  |

  Rule: Two loaded copies of the extension do not both connect

    # .omp/AGENTS.md: an installed plugin and a working-tree copy can both load.

    Scenario: The second copy defers to the first
      Given the installed plugin and the working-tree extension are both loaded in one omp process
      When both resolve agent credentials
      Then only the first copy to announce itself on "agent-bus:instance" registers the bus tools and opens the stream
      And the second copy logs that another copy is active
