@M1 @wip @rfc-5 @rfc-6
Feature: A project runs on one thread across three Domains
  As a team spread over 103mail.com, example.com and mail.acme.test
  We want one conversation that everyone sees the same way
  So that it does not matter where each of us hosts our mail

  Background:
    Given Alice, Ann and Arjen have accounts on 103mail.com
    And Bob, Carol and Chris have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  Scenario: Kick-off, replies from every Domain, and a thread that agrees everywhere
    When Alice writes to Ann, Bob, Carol and Erin: "Website relaunch" with the body "Kick-off on Monday. Please reply with your availability."
    Then Alice's send is confirmed within 2 seconds
    And within 30 seconds everyone sees:
      | person | folder | thread           | messages | unread |
      | Alice  | Inbox  | Website relaunch | 1        | no     |
      | Ann    | Inbox  | Website relaunch | 1        | yes    |
      | Bob    | Inbox  | Website relaunch | 1        | yes    |
      | Carol  | Inbox  | Website relaunch | 1        | yes    |
      | Erin   | Inbox  | Website relaunch | 1        | yes    |
    And Arjen, Chris and Eddie have no thread "Website relaunch"
    And 103mail.com delivered exactly one envelope to example.com and one to mail.acme.test

    When Bob replies to "Website relaunch": "Monday works for me"
    And Erin replies to "Website relaunch": "I can do Monday after 14:00"
    And Ann replies to "Website relaunch": "Booked room 3"
    Then within 30 seconds the thread "Website relaunch" reads the same for everyone:
      | from              | body                        |
      | alice@103mail.com | Kick-off on Monday. Please reply with your availability. |
      | bob@example.com   | Monday works for me         |
      | erin@mail.acme.test | I can do Monday after 14:00 |
      | ann@103mail.com   | Booked room 3               |
    And every member sees Alice, Ann, Bob, Carol and Erin as members of "Website relaunch"

    When Bob reads "Website relaunch"
    Then "Website relaunch" is read for Bob and still unread for Carol
    And read state never crossed a Domain: Carol, Erin and Ann still see it as unread

    When Carol replies to "Website relaunch": "Sorry, I'm out on Monday"
    Then within 30 seconds "Website relaunch" is unread again for Bob, Alice, Ann and Erin
    And each member has exactly one thread called "Website relaunch"

    And 103mail.com holds accounts only for its own people
    And example.com holds accounts only for its own people
    And mail.acme.test holds accounts only for its own people
    And no email has left any Domain containing:
      | Website relaunch               |
      | Kick-off on Monday             |
      | Monday works for me            |
      | Sorry, I'm out on Monday       |

  Scenario: Two independent conversations with the same subject stay apart
    When Alice writes to Bob and Erin: "Budget"
    And Carol writes to Alice and Eddie: "Budget"
    Then within 30 seconds everyone sees:
      | person | threads called "Budget" |
      | Alice  | 2                       |
      | Bob    | 1                       |
      | Carol  | 1                       |
      | Erin   | 1                       |
      | Eddie  | 1                       |
    When Bob replies to Alice's "Budget": "Q3 looks tight"
    Then within 30 seconds only Alice's thread with Bob and Erin shows "Q3 looks tight"
    And Carol and Eddie never see "Q3 looks tight"
    And every person's contact suggestions now include everyone they exchanged mail with
