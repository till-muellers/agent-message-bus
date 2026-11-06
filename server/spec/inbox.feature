Feature: Agent inbox
  As a leader or member agent
  I want a durable inbox that I can page through, acknowledge and stream live
  So that I never miss a message, even across disconnects

  # Tools: bus_inbox_read, bus_inbox_ack. Stream: GET /v1/inbox/stream.
  # The same member agent can hold one token per space; a member token only
  # covers the inbox entries of its own space, a leader sees all its spaces.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "alpha"
    And leader "lead" created space "beta"
    And leader "lead" invited member "worker" to space "alpha"
    And leader "lead" invited member "worker" to space "beta"
    And these inbox entries were stored, in this order:
      | recipient | space | type           | from    | body |
      | worker    | alpha | direct_message | someone | a1   |
      | worker    | beta  | direct_message | someone | b1   |
      | lead      | alpha | direct_message | someone | l1   |
      | worker    | alpha | direct_message | someone | a2   |
      | lead      | beta  | direct_message | someone | l2   |

  Rule: Entries have a fixed shape and six types

    Scenario: An entry exposes only its public fields
      When member "worker" reads its inbox with its "alpha" token
      Then every returned entry has the fields "id", "space", "type", "from", "critical", "class" and "createdAt"
      And the optional fields "topic", "blackboard", "threadId", "threadTitle" and "body" appear only when set
      And no entry exposes the recipient agent id or the space id

    Scenario: Stored entries get increasing ids and a UTC creation time
      Given the clock reads "2026-10-25T09:30:00.123456789+02:00"
      When these inbox entries are stored for "worker" in space "alpha" in one transaction:
        | type         | from | critical | blackboard | threadId | threadTitle | topic | body  |
        | thread_post  | lead | true     | plan       | 7        | design      |       | hello |
        | topic_closed | lead | false    |            |          |             | news  |       |
      Then both entries get ids greater than every earlier entry, in insertion order
      And both entries have createdAt "2026-10-25T07:30:00.123456789Z"
      And member "worker" reading its "alpha" inbox after entry "a2" gets exactly those two entries with all their fields

    Scenario Outline: Entry type "<type>"
      Then an inbox entry may have type "<type>"

      Examples:
        | type               |
        | direct_message     |
        | topic_message      |
        | topic_closed       |
        | thread_post        |
        | thread_removed     |
        | blackboard_removed |

  Rule: Reading the inbox (bus_inbox_read) pages entries without removing them

    Scenario: A member sees only the entries of its token's space
      When member "worker" reads its inbox with its "alpha" token
      Then the call returns entries "a1, a2" in id order

    Scenario: The same member with the other space's token sees that space only
      When member "worker" reads its inbox with its "beta" token
      Then the call returns entries "b1"

    Scenario: A leader sees its own entries in every space
      When leader "lead" reads its inbox
      Then the call returns entries "l1, l2" in id order

    Scenario: after skips entries up to and including the given id
      When member "worker" reads its inbox with its "alpha" token after the id of entry "a1"
      Then the call returns entries "a2"

    Scenario: after defaults to 0
      When member "worker" reads its inbox with its "alpha" token without an after cursor
      Then the call returns entries "a1, a2"

    Scenario: limit caps the page
      When leader "lead" reads its inbox with limit 1
      Then the call returns entries "l1"

    Scenario: The maximum limit is 500
      When leader "lead" reads its inbox with limit 500
      Then the call returns entries "l1, l2"

    Scenario: limit defaults to 100
      Given 101 more inbox entries were stored for "lead" in space "alpha"
      When leader "lead" reads its inbox without a limit
      Then the call returns 100 entries

    Scenario Outline: limit outside 1..500 is rejected (limit <limit>)
      When leader "lead" reads its inbox with limit <limit>
      Then the call fails with invalid "limit must be 1..500"

      Examples:
        | limit |
        | 501   |
        | -1    |
        | 0     |

    Scenario: An empty inbox reads as an empty list
      Given leader "lead" acknowledged its inbox up to the id of entry "l2"
      When leader "lead" reads its inbox
      Then the call returns an empty list

    Scenario: Reading does not remove entries
      When member "worker" reads its inbox with its "alpha" token
      And member "worker" reads its inbox with its "alpha" token again
      Then both calls return entries "a1, a2"

    Scenario: The admin reads its own inbox
      When the admin reads its inbox
      Then the call returns an empty list

  Rule: Acknowledging (bus_inbox_ack) permanently deletes entries with id <= upTo

    Scenario: upTo is required
      When member "worker" acknowledges its inbox with its "alpha" token without upTo
      Then the call fails with invalid "upTo is required"

    Scenario: A member acknowledges only entries of its token's space
      When member "worker" acknowledges its inbox with its "alpha" token up to the id of entry "l2"
      Then the call returns {"acked": 2}
      And member "worker" reading with its "alpha" token gets no entries
      And member "worker" reading with its "beta" token gets entries "b1"
      And leader "lead" reading its inbox gets entries "l1, l2"

    Scenario: A partial acknowledgement keeps later entries
      When member "worker" acknowledges its inbox with its "alpha" token up to the id of entry "a1"
      Then the call returns {"acked": 1}
      And member "worker" reading with its "alpha" token gets entries "a2"
      And member "worker" reading with its "beta" token gets entries "b1"
      And leader "lead" reading its inbox gets entries "l1, l2"

    Scenario: Acknowledging is idempotent
      When member "worker" acknowledges its inbox with its "beta" token up to the id of entry "l2"
      Then the call returns {"acked": 1}
      When member "worker" acknowledges its inbox with its "beta" token up to the id of entry "l2" again
      Then the call returns {"acked": 0}
      And member "worker" reading with its "beta" token gets no entries
      And member "worker" reading with its "alpha" token gets entries "a1, a2"
      And leader "lead" reading its inbox gets entries "l1, l2"

    Scenario: A leader acknowledges only its own entries, across all its spaces
      When leader "lead" acknowledges its inbox up to the id of entry "l2"
      Then the call returns {"acked": 2}
      And leader "lead" reading its inbox gets no entries
      And member "worker" reading with its "alpha" token gets entries "a1, a2"
      And member "worker" reading with its "beta" token gets entries "b1"

    Scenario: The admin acknowledges its own inbox
      When the admin acknowledges its inbox up to the id of entry "l2"
      Then the call returns {"acked": 0}

  Rule: Stored entries wake their recipients only after the transaction commits

    Scenario: Delivering entries wakes each recipient once
      Given listeners on the live channels of "worker" and "lead"
      When an operation stores two entries for "worker" and one for "lead" and commits
      Then each listener receives exactly one empty wake-up signal

    Scenario: A failed operation stores and wakes nothing
      Given a listener on the live channel of "worker"
      When an operation produces an entry for "worker" and then fails
      Then "worker" has no new inbox entry
      And the listener receives no signal

  Rule: The inbox stream (GET /v1/inbox/stream) sends the backlog, then live entries

    # Server-sent events. Each entry is one frame:
    #   id: <entry id>\nevent: inbox\ndata: <entry JSON>\n\n
    # While idle the server sends a heartbeat comment ": ping\n\n" every 20 seconds.
    # Live publications are only wake-up signals: on each one the stream re-reads
    # the inbox after the last id it sent, so every entry goes out exactly once
    # and in id order. The stream subscribes before reading the backlog, so
    # nothing published in between is lost.

    Scenario: Stream response headers
      When member "worker" opens the inbox stream with its "alpha" token
      Then the response status is 200
      And the "Content-Type" header is "text/event-stream"
      And the "Cache-Control" header is "no-store"

    Scenario: Frames carry the entry id, the event name and the entry JSON
      When member "worker" opens the inbox stream with its "alpha" token
      Then each inbox frame has event "inbox"
      And each frame id equals the id inside its entry JSON

    Scenario: Heartbeats start once the backlog is sent
      Given the stream heartbeat interval is 1 millisecond
      When member "worker" opens the inbox stream with its "alpha" token
      Then the stream sends entries "a1, a2"
      And then the stream sends a ": ping" heartbeat

    Scenario: Backlog then a live entry
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2" and a heartbeat
      When entry "a3" for "worker" in space "alpha" is stored and published
      Then the stream sends entries "a3"

    Scenario: A live copy of a backlog entry is not sent again
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2" and a heartbeat
      When entry "a2" is published again
      And entry "a3" for "worker" in space "alpha" is stored and published
      Then the stream sends entries "a3"

    Scenario: A member stream skips live entries of other spaces
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2" and a heartbeat
      When entry "b2" for "worker" in space "beta" is stored and published
      And entry "a3" for "worker" in space "alpha" is stored and published
      Then the stream sends entries "a3"

    Scenario: A leader stream covers every space
      Given the stream heartbeat interval is 1 millisecond
      And leader "lead" opened the inbox stream
      And the stream sent entries "l1, l2" and a heartbeat
      When entry "l3" for "lead" in space "beta" is stored and published
      Then the stream sends entries "l3"

    Scenario: Entries published out of order still stream once, in id order
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2" and a heartbeat
      When entries "a3" and "a4" for "worker" in space "alpha" are stored
      And entry "a4" is published
      And entry "a3" is published
      And entry "a5" for "worker" in space "alpha" is stored and published
      Then the stream sends entries "a3, a4, a5"

    Scenario: The stream ends cleanly when the server shuts down
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "beta" token
      And the stream sent entries "b1" and a heartbeat
      When the server cancels all streams
      Then the stream is closed without error

    Scenario: The stream ends when the client disconnects
      Given member "worker" opened the inbox stream with its "alpha" token
      When the client closes the connection
      Then the server stops the stream and drops its live subscription

    Scenario: The stream ends when the live subscription is dropped for being slow
      Given member "worker" opened the inbox stream with its "alpha" token
      When the live channel drops the stream's subscription because its buffer is full
      Then the server ends the stream

    Scenario Outline: The stream ends when its token stops authenticating (<how>)
      # The stream re-checks its token after reading and before sending, on every
      # wake-up and every page, and on every heartbeat, so a dead token gets nothing
      # stored after the revocation, even when the same name is invited again into
      # the same space.
      Given member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2"
      When <how>
      And leader "lead" invites "worker" into space "alpha" again with task "secret task"
      Then the server ends the stream without sending "secret task"
      And the new token of "worker" can open the inbox stream

      Examples:
        | how                                          |
        | leader "lead" revokes member "worker"        |
        | member "worker" leaves space "alpha"         |
        | leader "lead" deletes space "alpha" and creates it again |

    Scenario: The bootstrap admin's stream ends when the admin token is rotated
      Given the admin joined space "alpha" and opened the inbox stream
      When the instance admin token is rotated
      And leader "lead" sends a direct message "secret after rotation" to "admin" in space "alpha"
      Then the server ends the stream without sending "secret after rotation"

    Scenario: The stream of a revoked admin ends before the re-minted admin's mail
      Given admin "ops" joined space "alpha" and opened the inbox stream
      When admin "admin" revokes admin "ops" and mints "ops" again
      And the new "ops" joins space "alpha"
      And leader "lead" sends a direct message "secret for ops" to "ops" in space "alpha"
      Then the server ends the stream without sending "secret for ops"
      And the new token of "ops" can open the inbox stream

    Scenario Outline: An idle stream ends at once when its credential dies (<how>)
      # Whatever deletes the tokens or participations (revoke, leave, delete space,
      # revoke admin, admin token rotation) wakes the agent's streams and waits after
      # the commit; the woken stream re-checks its token, so the slot is free again
      # within about a second instead of at the next heartbeat. Nothing is sent to it.
      Given <who> opened the inbox stream and nothing was sent to it
      When <how>
      Then the server ends the stream within 2 seconds
      And the agent's slot is free for the new token

      Examples:
        | who                | how                                           |
        | member "worker"    | leader "lead" revokes member "worker"         |
        | member "worker"    | member "worker" leaves space "alpha"          |
        | member "worker"    | leader "lead" deletes space "alpha"           |
        | admin "ops"        | admin "admin" revokes admin "ops"             |
        | the bootstrap admin | the instance admin token is rotated          |

    Scenario: A revocation just after the stream's token check still stops the next entry
      # The check follows the read, so an entry stored after the revocation is
      # read only after the revocation committed and the next check sees it.
      Given member "worker" opened the inbox stream with its "alpha" token
      When "worker" is revoked right after the stream's token check passed
      And an entry "leak" is stored for "worker" in space "alpha"
      Then the server ends the stream without sending "leak"

    Scenario: A wake-up while a page goes out still re-checks the token with nothing to send
      # The wake-up is merged into the next read, which finds nothing in the old
      # scope (mail of a space created again gets a new id); the check still runs.
      Given member "worker" opened the inbox stream with its "alpha" token
      When "worker" is revoked right after the stream's token check passed
      And an entry is stored for "worker" in space "beta" while the backlog goes out
      Then the server ends the stream
      And the stream slot of "worker" is free

    Scenario: An idle stream ends within one heartbeat of its token stopping to authenticate
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      When leader "lead" revokes member "worker"
      Then the server ends the stream
      And the stream slot of "worker" is free

    Scenario: The stream drops a client that stops reading
      # One frame write may stay blocked for at most 30 seconds. It blocks only
      # once the connection buffers are full, so an idle stream to a stalled
      # client lasts until its heartbeats fill them. The client reconnects and
      # gets the unacknowledged backlog again.
      Given the frame write timeout is 200 milliseconds
      And member "worker" opened the inbox stream from a client that never reads
      When more entries are stored for "worker" than the connection buffers hold
      Then the server ends the stream
      And the stream slot of "worker" is free
      And the server logs "inbox stream dropped: client not reading" at info level

    Scenario: Shutdown ends a stream blocked writing to a client that stops reading
      # A write in flight gets 250 milliseconds to finish, so a reading client still
      # sees a clean end; a blocked one fails then.
      Given member "worker" opened the inbox stream from a client that never reads
      And the stream is blocked writing its backlog
      When the server cancels all streams
      Then the server ends the stream within a second
      And the stream slot of "worker" is free
      And the server logs nothing about the stream

    Scenario: A fast reader keeps its stream while senders burst during a long backlog
      # The stream discards the wake-up signals queued behind each page it has read,
      # since its next read covers them, so the subscription overflows only when more
      # signals than its buffer arrive while a single page is sent.
      Given the live channel buffers 16 signals per subscription
      And 3000 entries for "worker" in space "alpha" are stored
      And member "worker" opened the inbox stream with its "alpha" token
      When 8 wake-up signals are published while each page of the backlog streams
      Then the stream sends every entry
      And then entry "after the burst" for "worker" in space "alpha" is stored, published and streamed

    Scenario: A subscription dropped during the backlog ends the stream after the current page
      Given the live channel buffers 16 signals per subscription
      And member "worker" opened the inbox stream with its "alpha" token
      When 17 wake-up signals are published while the backlog streams
      Then the stream sends entries "a1, a2"
      And the server ends the stream without an error

    Scenario: Reconnecting replays every unacknowledged entry
      Given member "worker" streamed entries "a1, a2" and disconnected without acknowledging
      When member "worker" opens the inbox stream with its "alpha" token again
      Then the stream sends entries "a1, a2"

    Scenario: Acknowledged entries are not replayed
      Given member "worker" acknowledged its inbox with its "alpha" token up to the id of entry "a1"
      When member "worker" opens the inbox stream with its "alpha" token
      Then the stream sends entries "a2"

    Scenario: The admin opens its inbox stream
      When the admin opens the inbox stream
      Then the response status is 200

    Scenario: The stream handler without an authenticated caller
      # Unreachable through the server routes, which authenticate first.
      When the inbox stream handler runs without an authenticated caller
      Then the call fails with unauthenticated "bearer token required"

  Rule: One live inbox stream per agent

    # One identity per agent: each agent has one stream slot, held from the
    # moment its stream is accepted until that stream ends for any reason
    # (client disconnect, server shutdown, a dropped live subscription, a token
    # that stopped authenticating or a client that stopped reading).
    # The slot is per agent, not per token, so a member's space tokens share it.
    # Tool calls are not restricted.

    Scenario: A second stream for the same agent is rejected with conflict
      Given member "worker" opened the inbox stream with its "alpha" token
      When member "worker" opens the inbox stream with its "beta" token
      Then the response status is 409
      And the call fails with conflict "agent worker already has an active inbox stream"

    Scenario: The first stream keeps delivering after a second one is rejected
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "alpha" token
      And the stream sent entries "a1, a2"
      And a second inbox stream for member "worker" was rejected with conflict
      When entry "a3" for "worker" in space "alpha" is stored and published
      Then the first stream sends entries "a3"

    Scenario: The slot is released when the stream's client disconnects
      Given member "worker" opened the inbox stream with its "alpha" token
      When the client closes the connection
      Then member "worker" can open the inbox stream with its "alpha" token again once the server notices the disconnect

    Scenario: The slot is released when the server cancels the stream
      Given the stream heartbeat interval is 1 millisecond
      And member "worker" opened the inbox stream with its "beta" token
      And the stream sent entries "b1" and a heartbeat
      When the server cancels that stream and it closes
      Then member "worker" can open the inbox stream with its "beta" token again

    Scenario: A member and a leader each have their own slot
      Given member "worker" opened the inbox stream with its "alpha" token
      When leader "lead" opens the inbox stream
      Then the response status is 200

    Scenario: A leader and a member each have their own slot
      Given leader "lead" opened the inbox stream
      When member "worker" opens the inbox stream with its "alpha" token
      Then the response status is 200

  Rule: The live channel fans out per recipient and drops slow listeners

    # In-process broker; the server sizes each listener's buffer at 256 messages.

    Scenario: Every listener of a channel receives each publication
      Given two listeners on channel "s/1" and one listener on channel "s/2"
      When "hi" is published on channel "s/1"
      Then both "s/1" listeners receive "hi"
      And the "s/2" listener receives nothing

    Scenario: Cancelling a listener stops delivery and is idempotent
      Given a listener on channel "t"
      When the listener is cancelled twice
      And "x" is published on channel "t"
      Then the listener's channel is closed without delivering "x"

    Scenario: A listener with a full buffer is closed and removed
      Given a listener on channel "t" with a buffer of 1
      When "1" and then "2" are published on channel "t"
      Then the listener receives "1"
      And then the listener's channel is closed
      And publishing "3" on channel "t" neither blocks nor fails
