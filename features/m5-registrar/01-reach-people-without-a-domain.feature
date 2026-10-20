@M5 @wip @rfc-8
Feature: People with only ordinary email join federated conversations
  As people on federated Domains
  We want to include Dave and Dana, who only have ordinary email, in our threads
  So that federation never limits who we can talk to

  103mail.com is the Registrar. example.com and mail.acme.test are not, and use
  103mail.com as their Default Registrar.

  Background:
    Given 103mail.com is a Registrar
    And example.com and mail.acme.test use 103mail.com as their Default Registrar
    And Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin has an account on mail.acme.test
    And Dave and Dana have no account on any Domain

  Scenario: Messages from three Domains wait for Dave and Dana, who then join the thread
    When Bob writes to Carol, Alice, Erin, "dave@gmail.test" and "dana@gmail.test": "Project kickoff" with the body "Slides attached" attaching "slides.pdf" (3 MB)
    Then within 30 seconds Carol, Alice and Erin have "Project kickoff"
    And exactly one invitation email went to "dave@gmail.test" and one to "dana@gmail.test"
    And both invitations are from 103mail.com and contain a link to claim a mailbox
    And no email has left any Domain containing:
      | Project kickoff |
      | Slides attached |
    And Bob has no "Delivery failed" thread

    When Erin writes to "dave@gmail.test" and Ann: "Side topic"
    And Alice replies to "Project kickoff": "Welcome Dave and Dana"
    Then no second invitation email goes to "dave@gmail.test" within 1 minute

    When Dave follows his invitation link and sets a password
    Then Dave's Inbox on 103mail.com shows:
      | thread          | messages | from (first message)   |
      | Project kickoff | 2        | bob@example.com        |
      | Side topic      | 1        | erin@mail.acme.test    |
    And Dave can download "slides.pdf", byte-identical to Bob's

    When Dave replies to "Project kickoff": "Count me in"
    Then within 30 seconds Bob, Carol, Alice and Erin see "Count me in" from "dave@gmail.test"
    And Dana's waiting mailbox now holds 3 messages on "Project kickoff"

    When Dana follows her invitation link and sets a password
    Then the thread "Project kickoff" reads the same for Bob, Carol, Alice, Erin, Dave and Dana

  Scenario: Invitation links are single-use and expire
    Given Bob wrote to "dave@gmail.test" and "dana@gmail.test": "Hello"
    When Dave claims his mailbox
    And anyone follows Dave's invitation link again
    Then they are told the link has already been used
    And Dave's mailbox is unchanged
    Given 31 days pass on the test clock
    When Dana follows her invitation link
    Then she is told the link has expired
    When Dana asks for a new invitation at "dana@gmail.test"
    Then a new invitation arrives and her waiting mail is still there when she claims it

  Scenario: A non-registrar Domain will not start without a Default Registrar
    Given mail.acme.test is configured with no Default Registrar
    When mail.acme.test starts
    Then it refuses to start, naming the missing Default Registrar
