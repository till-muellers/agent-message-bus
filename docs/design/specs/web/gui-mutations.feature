Feature: Web GUI mutations
  As a human operator of a bus
  I want to invite a member, revoke one, post and send from the web GUI
  So that I can steer a team without leaving the browser

  # Phase 2 of the web GUI. The operator signs in with an admin token, and an
  # admin acts as a leader in every space without joining it (see
  # docs/design/specs/admin/admin-role.feature). The web GUI is therefore not a
  # second, weaker authority next to the agent tools — it is the same
  # authority, driven from a browser.
  #
  # Admins are a strict superset of leaders: an admin may do everything a
  # leader may, and more — it revokes leaders as well, and it may delete any
  # space. The web GUI offers exactly the mutations an admin is entitled to.
  #
  # The web GUI is session-authenticated with a cookie, so every mutation
  # carries a CSRF token that the server checks. Without it the request is
  # refused, even with a valid session.
  #
  # The web GUI offers these mutations:
  #   POST /web/api/spaces/{space}/invites   create an invite code
  #   POST /web/api/spaces/{space}/revoke    revoke a member or a leader
  #   POST /web/api/spaces/{space}/threads/{id}/posts   post to a thread
  #   POST /web/api/spaces/{space}/topics/{topic}/messages  send to a topic
  #   DELETE /web/api/spaces/{space}          delete the space
  #
  # Admin management (mint, list, revoke admins) is the separate area in
  # gui-admins.feature.
  #
  # Deleting a board or a thread stays in the CLI and the agent tools: it is
  # irreversible and needs no browser.
  #
  # Cancelling a pending invite is a GUI-only web API endpoint, specified in
  # gui-invite-cancel.feature (PO-3). There is no cancel tool.
  #
  # Audit (PO-1): every GUI mutation writes one server log line with the admin name,
  # the space, the target and the action. There is no audit table and no actions list.

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"
    And the bootstrap admin "admin" exists
    And the admin minted admin "ops"
    And the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "worker" to space "demo"
    And member "worker" created blackboard "b" in space "demo"
    And member "worker" created thread "plan" with body "first" on blackboard "b" in space "demo"
    And leader "lead" created topic "notes" in space "demo"
    And the operator signed in with the token of admin "ops"

  Rule: A mutation needs a CSRF token

    Scenario: Creating an invite without a CSRF token is refused
      When the operator posts an invite for "new" in space "demo" without a CSRF token
      Then the response status is 403
      And no invite code is stored

    Scenario: Revoking without a CSRF token is refused
      When the operator posts a revoke of "worker" in space "demo" without a CSRF token
      Then the response status is 403
      And member "worker" is still a participant of space "demo"

    Scenario: Posting without a CSRF token is refused
      When the operator posts a reply to thread 1 in space "demo" without a CSRF token
      Then the response status is 403
      And thread 1 in space "demo" has one post

    Scenario: Sending without a CSRF token is refused
      When the operator posts a message to topic "notes" in space "demo" without a CSRF token
      Then the response status is 403
      And no inbox entry about topic "notes" was created

    Scenario: The CSRF token is bound to the session
      Given a second operator session is signed in with the admin token
      When the operator posts an invite for "new" in space "demo" with the CSRF token of the second session
      Then the response status is 403
      And no invite code is stored

  Rule: The operator creates an invite

    Scenario: The operator creates an invite code in any space
      When the operator posts an invite for "new" in space "demo"
      Then the response status is 200
      And the page shows the invite code once
      And the code is accepted

    Scenario: The invite code is shown once and never again
      When the operator posts an invite for "new" in space "demo"
      And the operator reloads the space "demo"
      Then the page does not show the invite code
      And the page lists "new" as an open invite

    Scenario: The operator sees an invite of a space it does not participate in
      Given the admin minted leader "lead2"
      And leader "lead2" created space "other"
      When the operator posts an invite for "new" in space "other"
      Then the response status is 200
      And the page shows the invite code once

    Scenario: An invalid member name is refused
      When the operator posts an invite for "Bad Name" in space "demo"
      Then the response status is 400
      And the page shows the error "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"
      And no invite code is stored

    Scenario: The name of an admin cannot be invited
      When the operator posts an invite for "ops" in space "demo"
      Then the response status is 409
      And the page shows the error "agent ops is an admin agent"
      And no invite code is stored

  Rule: The operator revokes a member

    Scenario: The operator revokes a member of any space
      When the operator posts a revoke of "worker" in space "demo"
      Then the response status is 200
      And member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

    Scenario: Revoking a member that is not there reports it
      When the operator posts a revoke of "ghost" in space "demo"
      Then the response status is 200
      And the page shows that nothing was removed

    Scenario: The operator revokes a leader of a space
      Given leader "lead2" joined space "demo"
      When the operator posts a revoke of "lead2" in space "demo"
      Then the response status is 200
      And the participants of space "demo" are:
        | name   | role   |
        | lead   | leader |
        | worker | member |

    Scenario: The operator revokes the last leader of a space
      Given leader "lead2" joined space "demo"
      And the operator revoked leader "lead2" from space "demo" with the confirmation
      When the operator posts a revoke of "lead" in space "demo" with the confirmation
      Then the response status is 200
      And the participants of space "demo" are:
        | name   | role   |
        | worker | member |
      And space "demo" still exists with no leader
      And the token of leader "lead" still authenticates

    Scenario: Revoking the last participant deletes the space
      # No empty spaces (docs/design/specs/admin/no-empty-spaces.feature).
      Given leader "lead2" created space "solo"
      When the operator posts a revoke of "lead2" in space "solo" with the confirmation
      Then the response status is 200
      And the space "solo" no longer exists

    Scenario: The operator revokes a member of a space it is not in
      Given leader "lead" created space "other"
      And leader "lead" invited member "helper" to space "other"
      When the operator posts a revoke of "helper" in space "other" with the confirmation
      Then the response status is 200
      And member "helper" asks who it is with its "other" token
      Then the call fails with unauthenticated "invalid token"

  Rule: The operator confirms a revoke

    # Revoking a member deletes its token for the space, its inbox entries there and
    # its blackboards. Revoking a leader removes only its participation and inbox
    # entries in the space; its leader token and boards stay. An admin can do either
    # in any space, so the web GUI asks for a confirmation that names the space, the
    # target and its role, under the admin's own name.

    Scenario: A revoke without a confirmation is refused
      When the operator posts a revoke of "worker" in space "demo" without a confirmation
      Then the response status is 400
      And the page asks to confirm the revoke of "worker" in space "demo"
      And member "worker" is still a participant of space "demo"

    Scenario: A confirmed revoke goes through
      When the operator posts a revoke of "worker" in space "demo" with the confirmation
      Then the response status is 200
      And member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

    Scenario: Every web GUI mutation writes one log line
      When the operator posts a revoke of "worker" in space "demo" with the confirmation
      Then the server logs one info line with admin "ops", space "demo", target "worker" and action "revoke"
      And the line contains no token

  Rule: The operator posts to a thread

    Scenario: The operator posts to a thread of any space
      When the operator posts a reply to thread 1 in space "demo"
      Then the response status is 200
      And the post of thread 1 has author "ops" and body "hello"
      And member "worker" receives a "thread_post" inbox entry from "ops" with threadId 1

    Scenario: A post body that is empty is refused
      When the operator posts an empty reply to thread 1 in space "demo"
      Then the response status is 400
      And the page shows the error "body must be 1..65536 bytes"

  Rule: The operator sends to a topic

    Scenario: The operator sends to a topic of any space
      Given member "worker" subscribed to topic "notes" in space "demo"
      When the operator posts the message "hi" to topic "notes" in space "demo"
      Then the response status is 200
      And member "worker" receives a "topic_message" inbox entry from "ops" with topic "notes" and body "hi"

    Scenario: The operator sends to the all topic of any space
      When the operator posts the message "hi" to topic "all" in space "demo"
      Then the response status is 200
      And member "worker" receives a "topic_message" inbox entry from "ops" with topic "all" and body "hi"

    Scenario: The operator sends a critical message
      When the operator posts the critical message "now" to topic "notes" in space "demo"
      Then the response status is 200
      And member "worker" receives a critical inbox entry from "ops"

  Rule: The operator deletes a space

    # PO-4: the GUI deletes spaces behind a confirmation step that names the space.

    # An admin may delete any space, even one that still has other leaders.

    Scenario: The operator deletes a space
      When the operator deletes space "demo" with the confirmation
      Then the response status is 200
      And the space "demo" no longer exists
      And member "worker" asks who it is
      Then the call fails with unauthenticated "invalid token"

    Scenario: The operator deletes a space that still has other leaders
      Given leader "lead2" joined space "demo"
      When the operator deletes space "demo" with the confirmation
      Then the response status is 200
      And the space "demo" no longer exists

    Scenario: A delete without a confirmation is refused
      When the operator deletes space "demo"
      Then the response status is 400
      And the page asks to confirm the deletion of space "demo"
      And the space "demo" still exists

    Scenario: A delete writes one log line
      When the operator deletes space "demo" with the confirmation
      Then the server logs one info line with admin "ops", space "demo", target "demo" and action "delete-space"

    Scenario: Deleting without a session is refused
      Given the operator signed out
      When the operator deletes space "demo" with the confirmation
      Then the response status is 401
      And the space "demo" still exists

  Rule: A mutation never runs without a session

    Scenario: Creating an invite without a session is refused
      Given the operator signed out
      When the operator posts an invite for "new" in space "demo"
      Then the response status is 401
      And no invite code is stored

    Scenario: Revoking without a session is refused
      Given the operator signed out
      When the operator posts a revoke of "worker" in space "demo"
      Then the response status is 401
      And member "worker" is still a participant of space "demo"

    Scenario: Posting without a session is refused
      Given the operator signed out
      When the operator posts a reply to thread 1 in space "demo"
      Then the response status is 401
      And thread 1 in space "demo" has one post
