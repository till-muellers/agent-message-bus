Feature: Delivery receipts and presence
  As a participant of a space
  I want to know whether my message reached a connected agent or waits in its inbox
  So that I can tell "delivered" from "queued" the way omp's peer messaging does

  # Server-side change. Native reference: omp's send result is "Delivered to <id>.",
  # "Queued for <id> (was parked; revived)." or "Failed: <id> is not running."
  # (irc/messaging.ts:91-101) and child prompts list peers with their status
  # (prompts/system/subagent-system-prompt.md:27-49).
  # The bus already knows who is connected: one live inbox stream slot per agent
  # (spec/inbox.feature, Rule "One live inbox stream per agent"; internal/inbox/slots.go).
  # "connected" below means: the agent holds its inbox stream slot, or it is blocked in a
  # server bus_wait call (docs/design/specs/mail/bus-wait.feature), which also receives a new entry
  # at once. Both are known to the inbox service in memory.
  # Delivery stays durable: a queued entry is delivered when the agent connects.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "peer" to space "alpha"

  Rule: Send results carry a receipt per recipient

    # "ok": true stays in the result so existing clients keep working.

    Scenario: A direct message to a connected agent is live
      Given member "peer" opened the inbox stream
      When member "worker" sends a direct message to "peer"
      Then the call returns {"ok": true, "receipts": [{"to": "peer", "delivery": "live"}]}

    Scenario: A direct message to an agent without a stream is queued
      When member "worker" sends a direct message to "peer"
      Then the call returns {"ok": true, "receipts": [{"to": "peer", "delivery": "queued"}]}
      And "peer" has the entry in its inbox

    Scenario: A topic message returns one receipt per subscriber except the sender
      Given members "peer" and "worker" and leader "lead" subscribed to topic "dev"
      And member "peer" opened the inbox stream
      When member "worker" sends a message to topic "dev"
      Then the call returns receipts for "lead" (queued) and "peer" (live), sorted by name

    Scenario: A topic without other subscribers returns no receipts
      Given only member "worker" subscribed to topic "solo"
      When member "worker" sends a message to topic "solo"
      Then the call returns {"ok": true, "receipts": []}

    Scenario: A post returns receipts for the thread's subscribers except the author
      Given member "peer" subscribed to thread 1
      When member "worker" adds a post to thread 1
      Then the call returns {"postId": <id>, "receipts": [{"to": "peer", "delivery": "queued"}]}

    Scenario: The receipt reflects the slot at commit time
      Given member "peer" opened the inbox stream
      When member "peer"'s stream ends while member "worker" sends a direct message to "peer"
      Then the receipt is "live" or "queued"
      And in both cases "peer" receives the entry exactly once, live or from the backlog

  Rule: Participant lists show who is connected

    Scenario: bus_list_participants marks connected agents
      Given member "peer" opened the inbox stream
      When member "worker" lists the participants of space "alpha"
      Then "peer" is listed with "connected": true
      And "worker" and "lead" are listed with "connected": false

    Scenario: Presence is per agent, not per space
      Given leader "lead" also leads space "beta"
      And leader "lead" opened the inbox stream
      When member "worker" lists the participants of space "alpha"
      Then "lead" is listed with "connected": true

  Rule: An agent waiting in bus_wait counts as connected

    Scenario: A direct message to an agent blocked in bus_wait is live
      Given member "peer" is blocked in a server bus_wait call without holding the stream slot
      When member "worker" sends a direct message to "peer"
      Then the call returns {"ok": true, "receipts": [{"to": "peer", "delivery": "live"}]}

    Scenario: bus_list_participants marks an agent in bus_wait as connected
      Given member "peer" is blocked in a server bus_wait call
      When member "worker" lists the participants of space "alpha"
      Then "peer" is listed with "connected": true
