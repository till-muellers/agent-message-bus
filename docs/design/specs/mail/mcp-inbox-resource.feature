Feature: MCP inbox resource with subscription push
  As an agent connected through an MCP client
  I want the bus to tell my client when mail arrives
  So that my client can wake me or inject a notice instead of me polling bus_inbox_read

  # LATER — a future version, NOT in the current version (PO, Q-M3). Kept as the design
  # for when standards-track MCP push is wanted. PROPOSAL (docs/design/specs/mail/analysis.md).
  #
  # The notice of mail-notice.feature is mapped onto standard MCP resources:
  #   resource URI  bus://_inbox/<agent>           leader or admin token (all its spaces)
  #                 bus://_inbox/<agent>/<space>   member token (its space)
  #   The "_inbox" segment cannot collide with a space name: names start with [a-z0-9]
  #   (the bus:// URL option, docs/design/specs/omp-parity/omp-bus-url.feature, uses bus://<space>/…).
  #   resources/list           lists exactly the caller's own inbox URI
  #   resources/read           returns the same JSON array as bus_inbox_read (first page)
  #   resources/subscribe      allowed only on the caller's own inbox URI
  #   notifications/resources/updated {uri}   sent on every notice (edge or subscribe-time level)
  #
  # Transport change: today /mcp is stateless with JSON responses (transports.feature),
  # so the go-sdk answers GET with 405 and drops each session after its request;
  # a resources/subscribe would be forgotten at once. Push needs MCP sessions:
  #   - protocol 2025-11-25 and older: stateful streamable HTTP (Mcp-Session-Id);
  #     notifications go out on the client's standalone GET stream.
  #   - protocol 2026-07-28 (SEP-2575, go-sdk v1.8.0): subscriptions/listen, a long-lived
  #     POST answered as SSE; works without sessions.
  # Tool calls behave exactly as before in both modes.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" invited member "worker" to space "alpha"

  Rule: The server advertises the inbox as a subscribable resource

    Scenario: Capabilities
      When a client initializes an MCP session at /mcp with leader "lead"'s token
      Then the server capabilities contain "resources" with "subscribe" true
      And the server capabilities still contain "tools"

    Scenario: A leader lists only its own inbox
      Given leader "lead" connected to /mcp with its token
      When leader "lead" lists the MCP resources
      Then the list holds exactly one resource with URI "bus://_inbox/lead"
      And its mimeType is "application/json"
      And its description tells the agent to call bus_inbox_read and bus_inbox_ack

    Scenario: A member lists the inbox of its token's space
      Given member "worker" connected to /mcp with its "alpha" token
      When member "worker" lists the MCP resources
      Then the list holds exactly one resource with URI "bus://_inbox/worker/alpha"

    Scenario: An admin lists its own inbox
      # Admins are named agents with their own inbox (docs/design/specs/admin/admin-messaging.feature).
      Given the bootstrap admin minted admin "ops"
      And admin "ops" connected to /mcp with its token
      When admin "ops" lists the MCP resources
      Then the list holds exactly one resource with URI "bus://_inbox/ops"

  Rule: Reading the inbox resource equals bus_inbox_read without arguments

    Scenario: Reading returns the first page of unacknowledged entries
      Given entries "a1, a2" for "worker" in space "alpha" are waiting
      And member "worker" connected to /mcp with its "alpha" token
      When member "worker" reads resource "bus://_inbox/worker/alpha"
      Then the text content is the JSON array of entries "a1, a2"
      And reading does not remove the entries

    Scenario Outline: Another agent's inbox cannot be read or subscribed (<method>)
      Given member "worker" connected to /mcp with its "alpha" token
      When member "worker" calls <method> with URI "bus://_inbox/lead"
      Then the call fails with a JSON-RPC error
      And nothing about the inbox of "lead" is revealed

      Examples:
        | method              |
        | resources/read      |
        | resources/subscribe |

  Rule: A subscribed session gets resources/updated on each notice

    Scenario: The first entry into an empty inbox pushes one update
      Given member "worker" opened a stateful MCP session with its "alpha" token
      And the session opened its standalone GET stream
      And the session subscribed to "bus://_inbox/worker/alpha"
      When entry "a1" for "worker" in space "alpha" is stored
      Then the GET stream carries one "notifications/resources/updated" with uri "bus://_inbox/worker/alpha"

    Scenario: Further entries before an acknowledgement push nothing
      Given member "worker"'s session is subscribed and was notified about entry "a1"
      When entry "a2" for "worker" in space "alpha" is stored
      Then the session receives no further notification

    Scenario: Acknowledging everything re-arms the push
      Given member "worker"'s session is subscribed and was notified about entry "a1"
      And member "worker" called MCP tool "bus_inbox_ack" with {"upTo": <id of a1>}
      When entry "a2" for "worker" in space "alpha" is stored
      Then the session receives one "notifications/resources/updated"

    Scenario: Subscribing to a non-empty inbox pushes one update at once
      Given entry "a1" for "worker" in space "alpha" is waiting
      And member "worker" opened a stateful MCP session with its "alpha" token
      When the session subscribes to "bus://_inbox/worker/alpha"
      Then the session receives one "notifications/resources/updated" right after the subscribe result

    Scenario: Unsubscribed sessions get nothing
      Given member "worker" opened a stateful MCP session with its "alpha" token without subscribing
      When entry "a1" for "worker" in space "alpha" is stored
      Then the session receives no notification

    Scenario: Another agent's entries do not reach the session
      Given member "worker"'s session is subscribed to "bus://_inbox/worker/alpha"
      When entry "l1" for "lead" in space "alpha" is stored
      Then the session receives no notification

  Rule: MCP sessions are bounded

    Scenario: An idle session expires
      Given member "worker" opened a stateful MCP session and then went silent
      When the session idle timeout passes
      Then the server closes the session and drops its subscriptions

    Scenario: A session is bound to the token's identity
      Given member "worker" opened a stateful MCP session with its "alpha" token
      When a request with leader "lead"'s token reuses that Mcp-Session-Id
      Then the HTTP request is rejected with status 403

    Scenario: A session ends when its token is revoked
      Given member "worker"'s session is subscribed to "bus://_inbox/worker/alpha"
      When leader "lead" revokes member "worker"
      Then the session's next request is rejected with status 401
      And no further notification is sent to it

  Rule: Clients on protocol 2026-07-28 listen without sessions

    Scenario: subscriptions/listen streams updates over one POST
      Given entries for "worker" in space "alpha" are acknowledged
      And member "worker" sent subscriptions/listen for resource "bus://_inbox/worker/alpha" on protocol "2026-07-28"
      And the response is an SSE stream that began with "notifications/subscriptions/acknowledged"
      When entry "a1" for "worker" in space "alpha" is stored
      Then the stream carries "notifications/resources/updated" with uri "bus://_inbox/worker/alpha"
      And its _meta carries the listen request id under "io.modelcontextprotocol/subscriptionId"

  Rule: Existing tool behaviour is unchanged

    # The go-sdk's stateful handler rejects a tools/call on a session that never
    # initialized ("method ... is invalid during session initialization"), while the
    # stateless handler accepts it. Requests without Mcp-Session-Id that are not
    # initialize (and not GET/DELETE) keep going to the stateless handler.

    Scenario: A one-shot tool call without initialize still works
      When a client calls MCP tool "bus_whoami" with leader "lead"'s token in a single POST without initializing
      Then the result is as in transports.feature
      And no MCP session is created

    Scenario: Tool calls inside a session behave as before
      Given leader "lead" opened a stateful MCP session with its token
      When leader "lead" calls MCP tool "bus_whoami" in that session
      Then the result is as in transports.feature
