@M1 @wip @rfc-4 @rfc-4.1.1
Feature: A subdomain and its parent are strangers
  As a company running acme.test and mail.acme.test as separate Domains
  We want each to federate on its own keys and speak only for its own people
  So that owning a parent name grants no power over a subdomain, or the reverse

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob has an account on example.com
    And Erin and Eddie have accounts on mail.acme.test
    And Pete has an account on acme.test

  Scenario: One thread with people on a parent and its subdomain
    When Alice writes to Bob, Erin, Eddie and Pete: "Acme integration"
    Then within 30 seconds Bob, Erin, Eddie and Pete have "Acme integration"
    And 103mail.com discovered mail.acme.test at webmail.mail.acme.test and acme.test at webmail.acme.test
    And the two descriptors carry different keys
    When Pete replies to "Acme integration": "Parent here"
    And Eddie replies to "Acme integration": "Subdomain here"
    Then within 30 seconds the thread "Acme integration" reads the same for everyone:
      | from                 | body           |
      | alice@103mail.com    | (kick-off)     |
      | pete@acme.test       | Parent here    |
      | eddie@mail.acme.test | Subdomain here |

  @protocol
  Scenario: Neither may speak for the other
    Given Alice wrote to Erin and Pete: "Who is who"
    When acme.test sends 103mail.com a reply to "Who is who" with the sender "erin@mail.acme.test"
    Then 103mail.com answers 401 "sender_mismatch"
    When mail.acme.test sends 103mail.com a reply to "Who is who" with the sender "pete@acme.test"
    Then 103mail.com answers 401 "sender_mismatch"
    And Alice still sees 1 message on "Who is who"

  @outage
  Scenario: A parent outage does not affect its subdomain
    When acme.test goes down
    And Alice writes to Erin and Pete: "Status"
    Then within 30 seconds Erin has "Status"
    And Pete has no "Status" until acme.test comes back
