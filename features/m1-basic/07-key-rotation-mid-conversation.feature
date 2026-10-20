@M1 @wip @rfc-4.3 @rfc-9.5
Feature: Domains change keys in the middle of a conversation
  As operators of federated Domains
  We want to rotate signing keys on our own schedule
  Without any member of an ongoing conversation noticing

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test
    And Alice wrote to Bob, Carol, Erin and Eddie: "Roadmap"
    And within 30 seconds everyone has "Roadmap", so every Domain has pinned the others' keys

  Scenario: A peer rotates and everyone keeps talking
    When example.com rotates its signing key
    And Bob replies to "Roadmap": "Signed with our new key"
    And Erin replies to "Roadmap": "Still on our old key"
    Then within 30 seconds everyone sees 3 messages on "Roadmap"
    And 103mail.com and mail.acme.test now pin example.com's new key

  @wip
  Scenario: We rotate our own key and stay trusted through the window
    When 103mail.com rotates its signing key
    Then 103mail.com's descriptor lists the old key as a previous key
    When Alice replies to "Roadmap": "New key here"
    Then within 30 seconds Bob, Carol, Erin and Eddie see "New key here"
    Given 49 hours pass on the test clock
    Then 103mail.com's descriptor no longer lists the old key
    When the test peer sends example.com a message signed with 103mail.com's old key
    Then example.com answers 401 "bad_signature"
    And Ann replies to "Roadmap": "Still fine"
    And within 30 seconds everyone sees "Still fine"

  @outage
  Scenario: A key rotation while a peer is down
    When mail.acme.test goes down
    And example.com rotates its signing key
    And Bob replies to "Roadmap": "Rotated while you were away"
    When mail.acme.test comes back
    Then within 3 minutes Erin and Eddie see "Rotated while you were away"
