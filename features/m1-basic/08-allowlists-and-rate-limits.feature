@M1 @wip @protocol @rfc-9.7 @rfc-9.10
Feature: Operators decide who may send and how fast
  As operators of private and public Domains
  We want a private Domain to talk only to peers it chose
  And every Domain to cap how fast one peer can send, without hurting the others

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  Scenario: A private Domain takes part in a group thread only with its chosen peers
    Given example.com only accepts mail from 103mail.com
    When Alice writes to Bob, Carol and Erin: "Partner call"
    Then within 30 seconds Bob, Carol and Erin have "Partner call"
    When Erin replies to "Partner call": "Can we move it to 15:00?"
    Then within 30 seconds Alice sees Erin's reply
    And Erin has "Delivery failed: Partner call" naming Bob and Carol and the reason "peer_not_allowed"
    And Bob and Carol never see Erin's reply
    When Bob replies to "Partner call": "15:00 is fine"
    Then within 30 seconds Alice and Erin see "15:00 is fine"

  @wip
  Scenario: A noisy peer is slowed down while the others are not
    Given example.com accepts at most 10 envelopes per minute from one peer
    And the test peer is known to example.com
    When the test peer sends Bob 30 messages within a minute
    And Alice writes to Bob and Carol: "Urgent" at the same time
    Then the test peer gets 10 accepted and the rest 429 "rate_limited" with a retry time
    And within 30 seconds Bob and Carol have "Urgent"
    And Bob has exactly 10 messages from the test peer
