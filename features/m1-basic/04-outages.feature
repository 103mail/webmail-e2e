@M1 @outage @rfc-6.4
Feature: Conversations survive Domains going down
  As people on several Domains
  We want mail to keep flowing between the Domains that are up
  And to catch up, in order and without duplicates, when a Domain returns

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test
    And Alice wrote to Ann, Bob, Carol, Erin and Eddie: "Release planning" with the body "Kick-off"

  Scenario: One Domain is down for an hour of conversation
    Given within 30 seconds everyone has "Release planning" with 1 message

    When example.com goes down
    And Alice replies to "Release planning": "Freeze on the 12th"
    And Erin replies to "Release planning": "QA needs two days"
    And Ann replies to "Release planning": "Docs are ready"
    Then every send is confirmed within 2 seconds
    And within 30 seconds Alice, Ann, Erin and Eddie see 4 messages on "Release planning"
    And mail between 103mail.com and mail.acme.test was not delayed by example.com

    When example.com comes back
    Then within 3 minutes Bob and Carol see "Release planning" with 4 messages
    And the thread "Release planning" reads the same for everyone:
      | from                | body               |
      | alice@103mail.com   | Kick-off           |
      | alice@103mail.com   | Freeze on the 12th |
      | erin@mail.acme.test | QA needs two days  |
      | ann@103mail.com     | Docs are ready     |
    And nobody has a duplicate message

    When Bob replies to "Release planning": "Sorry, we were down"
    Then within 30 seconds everyone sees 5 messages on "Release planning"

  Scenario: Two Domains are down at once and return in a different order
    When example.com goes down
    And mail.acme.test goes down
    And Alice replies to "Release planning": "Anyone there?"
    And Ann replies to "Release planning": "Still here"
    When mail.acme.test comes back
    Then within 3 minutes Erin and Eddie see 3 messages on "Release planning"
    And Bob and Carol still see 1 message
    When example.com comes back
    Then within 3 minutes everyone sees 3 messages on "Release planning", in the same order

  Scenario: A Domain that goes down while it is sending catches up on both directions
    When mail.acme.test goes down
    And Bob replies to "Release planning": "Ship it"
    And Carol replies to "Release planning": "Agreed"
    When mail.acme.test comes back
    And Erin replies to "Release planning": "Back online, ship it"
    Then within 3 minutes everyone sees 4 messages on "Release planning"
    And Erin's reply appears after Bob's and Carol's for everyone

  @wip
  Scenario: A peer that asks to slow down is respected
    Given example.com answers every delivery with "rate_limited" and a retry after 60 seconds
    When Alice replies to "Release planning": "Patience"
    And Erin replies to "Release planning": "Patience too"
    Then 103mail.com and mail.acme.test make no delivery attempt to example.com for 60 seconds
    When example.com stops rate limiting
    Then within 3 minutes Bob and Carol see both replies

  # A message that outlives the retry budget is not lost: the sender is told,
  # and it is sent again once the Domain answers (RFC 6.4).
  Scenario: A Domain down longer than the retry budget is backfilled when it returns
    Given the retry budget on 103mail.com is 2 minutes
    When mail.acme.test goes down
    And Alice replies to "Release planning": "Decision: ship on the 14th"
    Then within 30 seconds Ann, Bob and Carol see "Decision: ship on the 14th"
    And within 3 minutes Alice has "Delivery failed: Release planning" naming Erin and Eddie and the reason "timed_out"
    And the bounce says it will be sent again

    When mail.acme.test comes back
    Then within 2 minutes Erin and Eddie see "Decision: ship on the 14th"
    And within 30 seconds the thread "Release planning" reads the same for everyone:
      | from              | body                       |
      | alice@103mail.com | Kick-off                   |
      | alice@103mail.com | Decision: ship on the 14th |
    And nobody has a duplicate message
