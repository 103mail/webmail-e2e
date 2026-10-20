@M1 @protocol @rfc-5.4 @rfc-6.2 @rfc-6.3
Feature: A hostile peer cannot break into a live conversation
  As members of a conversation across Domains
  We want forged, replayed and malformed traffic to be turned away
  While the honest conversation carries on untouched

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin has an account on mail.acme.test
    And the test peer is known to 103mail.com and example.com
    And Alice wrote to Ann, Bob, Carol and Erin: "Acquisition"
    And Bob replied to "Acquisition": "Numbers attached tomorrow"
    And within 30 seconds everyone sees 2 messages on "Acquisition"

  Scenario: Every kind of forgery is refused and nobody's view changes
    When the test peer tries each of these against example.com, aimed at the "Acquisition" thread:
      | attempt                                                         | answer | error                  |
      | a reply signed with a key it does not publish                   | 401    | bad_signature          |
      | a reply whose body changed after signing                        | 401    | bad_signature          |
      | a reply claiming to come from 103mail.com                       | 401    | sender_mismatch        |
      | a reply from its own Domain with the sender "alice@103mail.com" | 401    | sender_mismatch        |
      | a reply dated 10 minutes ago                                    | 401    | stale_request          |
      | a reply reusing a nonce it used a minute ago                    | 401    | replay                 |
      | a reply with protocol version "9.0"                             | 400    | unsupported_version    |
      | a reply addressed to 101 recipients                             | 400    | schema_violation       |
      | a reply larger than example.com's envelope limit                | 413    | too_large              |
      | a reply sent as plain text                                      | 415    | unsupported_media_type |
      | a reply with a compressed body                                  | 415    | unsupported_media_type |
    Then each attempt gets the answer and error listed
    And after 30 seconds everyone still sees exactly 2 messages on "Acquisition"
    And the member list of "Acquisition" is unchanged for everyone

    When Carol replies to "Acquisition": "Legal is reviewing"
    Then within 30 seconds everyone sees 3 messages on "Acquisition"

  Scenario: Honest retries are absorbed, not duplicated
    Given example.com accepts the next envelope from 103mail.com but the answer is lost
    When Alice replies to "Acquisition": "Board meets Thursday"
    Then 103mail.com retries the delivery
    And within 2 minutes Bob and Carol each see "Board meets Thursday" exactly once
    When the test peer replays 103mail.com's last envelope to example.com with a fresh signature of its own
    Then example.com answers 401 "sender_mismatch"
    And Bob and Carol still see "Board meets Thursday" exactly once

  Scenario: A peer cannot join a thread it was never part of
    When the test peer sends example.com a reply to "Acquisition" from "mallory@peer.test"
    Then example.com refuses to add it to "Acquisition"
    And nobody sees a message from "mallory@peer.test"
    And "mallory@peer.test" is not a member of "Acquisition" for anyone

  Scenario: A refusal is final for the sender
    Given example.com will refuse 103mail.com's next envelope with 400 "malformed"
    When Ann replies to "Acquisition": "Draft is ready"
    Then within 30 seconds Ann has "Delivery failed: Acquisition" naming Bob and Carol and the reason "malformed"
    And 103mail.com makes no further delivery attempt for that envelope
    And Alice and Erin still see "Draft is ready"
