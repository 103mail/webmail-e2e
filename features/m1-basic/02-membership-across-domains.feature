@M1 @wip @rfc-5
Feature: People join and leave a thread from any Domain
  As members of a cross-domain thread
  We want invitations and departures to look the same on every Domain
  So that the member list can be trusted wherever it is read

  Background:
    Given Alice, Ann and Arjen have accounts on 103mail.com
    And Bob, Carol and Chris have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  Scenario: Invitations bring the full history, departures stop delivery
    Given Alice wrote to Bob and Erin: "Vendor selection" with the body "Shortlist: three vendors, decision by Friday"
    And Bob replied to "Vendor selection": "I prefer vendor B"

    When Bob invites Chris to "Vendor selection"
    Then within 30 seconds Chris has "Vendor selection" in his Inbox with 2 messages
    And Chris can read "Shortlist: three vendors, decision by Friday"
    And every member sees Alice, Bob, Erin and Chris as members of "Vendor selection"

    When Erin invites Eddie and Arjen to "Vendor selection"
    Then within 30 seconds Eddie and Arjen each have "Vendor selection" with 2 messages
    And every member sees Alice, Bob, Erin, Chris, Eddie and Arjen as members

    When Chris leaves "Vendor selection"
    And Alice replies to "Vendor selection": "Going with vendor B"
    Then within 30 seconds Bob, Erin, Eddie and Arjen see "Going with vendor B"
    And Chris does not see "Going with vendor B"
    And every remaining member sees Alice, Bob, Erin, Eddie and Arjen as members

    When Chris tries to reply to "Vendor selection": "Wait, one more thing"
    Then Chris's reply is refused because he is no longer a member
    And nobody sees "Wait, one more thing"

  Scenario: Nobody can remove someone else, and nobody can forward
    Given Alice wrote to Ann, Bob and Erin: "Salary review"
    When Bob tries to remove Erin from "Salary review"
    Then Bob is told that only Erin can remove herself
    And within 30 seconds every member still sees Alice, Ann, Bob and Erin as members
    When Bob asks for a way to forward "Salary review" to Carol
    Then the API offers no forward action, only an invitation
    And Carol has no thread "Salary review"

  Scenario: Quoting starts a new thread with no way back
    Given Alice wrote to Bob and Erin: "Board minutes" with the body "Item 4: the merger is approved"
    When Bob quotes "Item 4: the merger is approved" in a new message to Carol and Chris: "FYI"
    Then within 30 seconds Carol and Chris have the thread "FYI" containing the quoted text
    And Carol and Chris have no access to "Board minutes"
    And "Board minutes" still lists only Alice, Bob and Erin as members
