@M5 @wip @rfc-8.5
Feature: Waiting mail moves to the Domain a person already has
  As Dave, who has an account on mail.acme.test with the real email dave@gmail.test
  I want mail that waited for me at the Registrar to come to my own Domain
  So that I keep one Inbox and later mail comes straight to me

  Background:
    Given 103mail.com is a Registrar
    And example.com uses 103mail.com as its Default Registrar
    And Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Dave has the account "dave@mail.acme.test" with the confirmed real email "dave@gmail.test"
    And Eddie has an account on mail.acme.test
    And Bob wrote to Carol, Alice and "dave@gmail.test": "Project kickoff"
    And Alice wrote to Ann and "dave@gmail.test": "Office keys"

  Scenario: Claim, backfill, and every later message routed directly
    When Dave follows his invitation link and claims it as "dave@mail.acme.test"
    Then within 1 minute Dave's Inbox on mail.acme.test has "Project kickoff" and "Office keys"
    And 103mail.com no longer holds a waiting mailbox for "dave@gmail.test"
    And every member of "Project kickoff" now sees "dave@mail.acme.test" as a member

    When Bob replies to "Project kickoff": "Any news?"
    And Carol writes to "dave@gmail.test" and Eddie: "New topic"
    Then within 30 seconds Dave sees "Any news?" on mail.acme.test
    And within 30 seconds Dave and Eddie have "New topic" on mail.acme.test
    And no new invitation email goes to "dave@gmail.test"
    And 103mail.com did not create a new waiting mailbox

    When Dave replies to "Project kickoff": "Moved to my own Domain"
    Then within 30 seconds Bob, Carol and Alice see the reply from "dave@mail.acme.test"

  @protocol
  Scenario: A stolen invitation cannot redirect Dave's mail
    When the test peer claims Dave's waiting mail as "dave@peer.test" using Dave's invitation token
    Then the claim is refused
    And Dave's waiting mail is still on 103mail.com
    When Dave claims it as "dave@mail.acme.test"
    Then within 1 minute Dave has both threads on mail.acme.test
    And the test peer received nothing
