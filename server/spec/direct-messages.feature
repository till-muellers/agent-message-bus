Feature: Direct messages
  As a participant of a space
  I want to send a message straight to one other participant
  So that I can talk to a single agent without involving a topic

  # Tool: bus_send_direct (space, to, body required; critical optional, default false).
  # Checks run in this order: space resolution, body size, critical permission,
  # self-addressing, recipient lookup. A failed call delivers nothing.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"
    And leader "lead" invited member "ann" to space "demo"
    And leader "lead" invited member "bob" to space "demo"

  Scenario: A member sends a direct message to another member
    When member "ann" sends direct message "hi" to "bob" in space "demo"
    Then the call returns {"ok": true, "receipts": [{"to": "bob", "delivery": "queued"}]}
    And member "bob" receives a "direct_message" inbox entry from "ann" with space "demo", critical false and body "hi"
    And "ann" receives no inbox entry
    And "lead" receives no inbox entry

  Scenario: A direct message produces exactly one entry, for the recipient only
    When leader "lead" sends direct message "ping" to "ann" in space "demo"
    Then the call succeeds
    And member "ann" has exactly 1 inbox entry
    And "bob" receives no inbox entry
    And "lead" receives no inbox entry

  Scenario: A leader sends a critical direct message
    When leader "lead" sends critical direct message "now" to "bob" in space "demo"
    Then the call succeeds
    And member "bob" receives a "direct_message" inbox entry from "lead" with critical true and body "now"

  Scenario Outline: Only leaders may send critical direct messages
    When <role> "<sender>" sends critical direct message "x" to "<to>" in space "demo"
    Then <outcome>
    And "<to>" <delivery>

    Examples:
      | role   | sender | to  | outcome                                                              | delivery                                                        |
      | leader | lead   | bob | the call succeeds                                                    | receives a "direct_message" inbox entry from "lead" with critical true |
      | member | ann    | bob | the call fails with forbidden "only leaders may send critical messages" | receives no inbox entry                                         |

  Scenario Outline: The body must be between 1 and 65536 bytes
    When member "ann" sends a direct message with a body of <size> bytes to "bob" in space "demo"
    Then <outcome>

    Examples:
      | size  | outcome                                                    |
      | 0     | the call fails with invalid "body must be 1..65536 bytes"  |
      | 1     | the call succeeds                                          |
      | 65536 | the call succeeds                                          |
      | 65537 | the call fails with invalid "body must be 1..65536 bytes"  |

  Scenario: A participant cannot send a direct message to themselves
    When member "ann" sends direct message "x" to "ann" in space "demo"
    Then the call fails with invalid "cannot send a direct message to yourself"
    And "ann" receives no inbox entry

  Scenario: The recipient must be a participant of the space
    When member "ann" sends direct message "x" to "zed" in space "demo"
    Then the call fails with not_found "participant zed not in space demo"

  Scenario: A leader who is not a participant cannot be addressed
    Given the admin minted leader "lead2"
    When member "ann" sends direct message "x" to "lead2" in space "demo"
    Then the call fails with not_found "participant lead2 not in space demo"

  Scenario: The body is checked before the recipient
    When member "ann" sends a direct message with a body of 0 bytes to "zed" in space "demo"
    Then the call fails with invalid "body must be 1..65536 bytes"

  Scenario: The critical permission is checked before self-addressing and the recipient
    When member "ann" sends critical direct message "x" to "ann" in space "demo"
    Then the call fails with forbidden "only leaders may send critical messages"
    When member "ann" sends critical direct message "x" to "zed" in space "demo"
    Then the call fails with forbidden "only leaders may send critical messages"

  Scenario: Self-addressing is checked before the recipient lookup
    When leader "lead" sends direct message "x" to "lead" in space "demo"
    Then the call fails with invalid "cannot send a direct message to yourself"

  Scenario: The space must exist
    When member "ann" sends direct message "x" to "bob" in space "nope"
    Then the call fails with not_found "space nope not found"

  Scenario: The space gate runs before any message checks
    When member "ann" sends a direct message with a body of 0 bytes to "bob" in space "nope"
    Then the call fails with not_found "space nope not found"

  Scenario: The admin sends direct messages without joining
    When the admin sends direct message "x" to "bob" in space "demo"
    Then bob receives a "direct_message" inbox entry from "admin" with body "x"

  Scenario: A leader who is not a participant cannot send direct messages
    Given the admin minted leader "lead2"
    When leader "lead2" sends direct message "x" to "bob" in space "demo"
    Then the call fails with forbidden "not a participant of space demo"

  Scenario: A member token only works in the space it was issued for
    Given leader "lead" created space "other"
    When member "ann" sends direct message "x" to "lead" in space "other"
    Then the call fails with forbidden "token is not valid for space other"

  Scenario Outline: Space, recipient and body are required
    When member "ann" sends a direct message without "<field>"
    Then the call fails with invalid "<field> is required"

    Examples:
      | field |
      | space |
      | to    |
      | body  |
