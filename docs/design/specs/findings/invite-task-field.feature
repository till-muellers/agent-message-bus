# FINAL (PO-approved, round 2): domain.ValidText(field, v) is approved, with
# ValidBody and ValidTitle kept as wrappers. This file is the replacement for
# server/spec/invites.feature:34-42 (the "The task follows the message body
# rule" scenario outline) and the body/title rules it states are unchanged.
#
# Lines that change with this decision:
#   server/spec/invites.feature:34-42   -> the task outline now expects
#                                         "task must be 1..65536 bytes"
#   server/internal/invites/service_test.go:45-46
#                                        -> the two task cases expect the same
#   server/internal/domain/text.go       -> gains ValidText; ValidBody and
#                                           ValidTitle delegate to it
#   server/internal/domain/text_test.go  -> gains the ValidText cases
#   (no skill line: the skill is retired; the guide lists error codes, not messages,
#    docs/design/specs/guidance/guide-draft.md "Rules")
# Unchanged: server/spec/direct-messages.feature, topics.feature,
# blackboards.feature (they keep "body must be 1..65536 bytes"),
# server/TOOLS.md:110 (the task argument description stays "1..65536 bytes").

Feature: Invite code task validation
  As a leader of a space
  I want a rejected invite task to name the field I got wrong
  So that I can fix the call without guessing which argument was invalid

  # The task of an invite code is delivered to the new member as a direct message from
  # the inviter when the code is accepted (docs/design/specs/guidance/first-prompt.feature). It is limited like a
  # message body: 1..65536 bytes (see server/internal/domain, MaxBodyBytes).
  # The limit is shared with message bodies, but the error message must name the
  # field that was rejected: "task", not "body".
  # Every other field keeps its current message:
  #   message body   -> "body must be 1..65536 bytes"
  #   thread title   -> "title must be 1..200 characters"

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"

  Rule: An invalid invite task names the task field (bus_invite_member)

    Scenario Outline: The task follows the body limit but names itself (<case>)
      When leader "lead" invites member "worker" to space "demo" with task <task>
      Then the call fails with invalid "task must be 1..65536 bytes"
      And no invite code is stored

      Examples:
        | case     | task           |
        | empty    | ""             |
        | too long | of 65537 bytes |

    Scenario: A task at the limit is accepted
      When leader "lead" invites member "worker" to space "demo" with task "fix the bug"
      Then the call returns name "worker", space "demo" and an invite code

    Scenario: An invite without a task is accepted
      When leader "lead" invites member "worker" to space "demo"
      Then the call returns name "worker", space "demo" and an invite code

  Rule: The shared text limits stay field-aware everywhere

    # The same helper serves every text field, so a message body still reports
    # "body" and a thread title still reports "title". Only the invite task
    # changes its message.

    Scenario Outline: A message body still names the body field (<case>)
      Given leader "lead" invited member "worker" to space "demo"
      When member "worker" sends <body> directly to "lead" in space "demo"
      Then the call fails with invalid "body must be 1..65536 bytes"

      Examples:
        | case     | body           |
        | empty    | ""             |
        | too long | of 65537 bytes |

    Scenario: A thread title still names the title field
      Given leader "lead" created blackboard "plans" in space "demo"
      When leader "lead" created thread "" on blackboard "plans" in space "demo"
      Then the call fails with invalid "title must be 1..200 characters"
