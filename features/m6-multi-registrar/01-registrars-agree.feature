@M6 @wip @rfc-8.3 @rfc-8.4 @rfc-8.6
Feature: Several Registrars agree on one mailbox per person
  As people writing from different parts of the federation
  We want Dave to end up with one mailbox holding everything sent to him
  However many Registrars first received mail for him

  Background:
    Given 103mail.com and registrar.test are Registrars that list each other as peers
    And example.com uses 103mail.com as its Default Registrar and registrar.test as fallback
    And mail.acme.test uses registrar.test as its Default Registrar
    And Alice has an account on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test
    And Rita has an account on registrar.test
    And Dave and Dana have no account on any Domain

  Scenario: Mail for Dave arrives at both Registrars and converges on claim
    When Erin writes to Eddie and "dave@gmail.test": "From Acme"
    Then within 30 seconds registrar.test holds a waiting mailbox for "dave@gmail.test"
    When Bob writes to Carol and "dave@gmail.test": "From Example"
    Then 103mail.com finds registrar.test's waiting mailbox and sends "From Example" there
    And exactly one invitation email reaches "dave@gmail.test"

    Given registrar.test is unreachable for lookups
    When Alice writes to "dave@gmail.test": "From 103mail"
    Then 103mail.com starts its own waiting mailbox for "dave@gmail.test"
    When registrar.test becomes reachable again

    When Dave claims his mailbox through registrar.test's invitation
    Then within 1 minute Dave's single Inbox has "From Acme", "From Example" and "From 103mail"
    And 103mail.com no longer holds a waiting mailbox for "dave@gmail.test"
    When Bob, Erin and Alice each reply on their thread with Dave
    Then within 30 seconds Dave sees all three replies in that one Inbox

  @outage
  Scenario: A Default Registrar is down and the fallback takes over
    Given 103mail.com is down
    When Bob writes to Carol and "dana@gmail.test": "Fallback"
    Then within 1 minute an invitation from registrar.test reaches "dana@gmail.test"
    When 103mail.com comes back
    And Carol writes to "dana@gmail.test": "Second try"
    Then "Second try" goes to the same waiting mailbox on registrar.test
    And Dana still has exactly one invitation

  Scenario: A Registrar that is removed is forgotten within a minute
    Given 103mail.com has learned about waiting mailboxes from registrar.test
    When the operator of 103mail.com removes registrar.test from its Registrar peers
    Then within 60 seconds 103mail.com answers no lookup with anything learned from registrar.test
    And Bob's next message to "dave@gmail.test" starts a waiting mailbox on 103mail.com
