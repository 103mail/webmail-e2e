@M1 @rfc-6.3 @rfc-6.5
Feature: A message to many people is delivered where it can be and bounced where it cannot
  As a sender writing to people on several Domains
  I want every reachable recipient to get my message
  And a clear bounce for each one who could not, so I never believe a lost message arrived

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test
    And nobody called "nobody@example.com" has an account on example.com

  Scenario: Good and unknown recipients in one message
    When Alice writes to Ann, Bob, "nobody@example.com" and Erin: "Invitation"
    Then within 30 seconds Ann, Bob and Erin each have "Invitation" in their Inbox
    And within 30 seconds Alice has these bounces in her Inbox:
      | thread                      | from                      | failed recipient   | reason            |
      | Delivery failed: Invitation | mailer-daemon@103mail.com | nobody@example.com | unknown_recipient |
    And the bounces exist only on 103mail.com: Bob, Erin and Ann never see them
    And within 30 seconds every member of "Invitation" sees Alice, Ann, Bob, Erin as the people who received it

    When Bob replies to "Invitation": "Thanks, see you there"
    Then within 30 seconds Alice, Ann and Erin see the reply
    And Alice gets no further bounce for "Invitation"

  Scenario: Everyone on a Domain is unknown, the rest still get it
    Given nobody called "ghost@mail.acme.test" or "spook@mail.acme.test" has an account on mail.acme.test
    When Bob writes to Alice, Carol, "ghost@mail.acme.test" and "spook@mail.acme.test": "Party"
    Then within 30 seconds Alice and Carol have "Party" in their Inbox
    And within 30 seconds Bob has one bounce "Delivery failed: Party" naming both unknown addresses
    And it is from "mailer-daemon@example.com"

  @outage
  Scenario: A Domain that never comes back produces a bounce after the retry budget
    Given the retry budget on 103mail.com is 2 minutes
    When mail.acme.test goes down for good
    And Alice writes to Bob, Erin and Eddie: "Quarterly report"
    Then within 30 seconds Bob has "Quarterly report"
    And within 3 minutes Alice has "Delivery failed: Quarterly report" naming Erin and Eddie and the reason "timed_out"
    And 103mail.com makes no further delivery attempt for "Quarterly report"
    When Bob replies to "Quarterly report": "Got it"
    Then Bob's own Domain retries to mail.acme.test on its own budget, independent of 103mail.com
