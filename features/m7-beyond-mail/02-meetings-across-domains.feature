@M7 @wip @rfc-15
Feature: People invite each other to meetings across Domains
  As people with accounts on different Domains
  We want a meeting to reach everyone invited, wherever their account is
  And every change and every answer to reach everyone, stacked like messages in a thread
  So that a meeting means the same to everyone, and never leaves the system by email

  # These stories prove the calendar extension, §15 of the RFC. How a Domain
  # stores a meeting is ADR 22 in ../webmail-server/docs/adr/: every change is
  # a new revision and every answer a new answer, nothing is edited or deleted,
  # and deleting a meeting only hides it from the one who deleted it.
  #
  # Times in the tables are in the meeting's own zone, Europe/Amsterdam, unless
  # a column says UTC. Each person reads a meeting only through their own
  # Domain's calendar API.

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  @rfc-15.2 @rfc-15.5
  Scenario: An invitation reaches three Domains and every answer reaches everyone
    When Alice invites Ann, Bob and Erin to "Launch review" on 3 November 2026 from 09:00 to 10:00 Europe/Amsterdam
    Then within 30 seconds Ann, Bob and Erin each have "Launch review" on 3 November 2026 from 09:00 to 10:00, organized by Alice
    And everyone invited sees these answers to "Launch review":
      | attendee | answer    |
      | Alice    | yes       |
      | Ann      | no answer |
      | Bob      | no answer |
      | Erin     | no answer |

    # Each answer comes from the attendee's own Domain and reaches every
    # attendee's Domain, not only the organizer's.
    When Bob answers "yes" to "Launch review"
    And Erin answers "maybe" to "Launch review"
    And Ann answers "no" to "Launch review"
    Then within 30 seconds everyone invited sees these answers to "Launch review":
      | attendee | answer |
      | Alice    | yes    |
      | Ann      | no     |
      | Bob      | yes    |
      | Erin     | maybe  |

    # A changed answer is stacked on the earlier one, not written over it.
    When Erin answers "yes" to "Launch review"
    Then within 30 seconds everyone invited sees Erin's answer to "Launch review" as "yes"
    And Erin's answers to "Launch review" read the same for everyone invited:
      | answer |
      | maybe  |
      | yes    |
    And Carol and Eddie have no meetings
    And no email has left any Domain containing:
      | Launch review |
    And example.com holds accounts only for its own people
    And mail.acme.test holds accounts only for its own people

  @rfc-15.3
  Scenario: Moving and cancelling a meeting stack as its history, and only the organizer can do either
    Given Alice invited Ann, Bob and Erin to "Budget" on 4 November 2026 from 14:00 to 15:00 Europe/Amsterdam
    And Bob answered "yes" to "Budget"
    And within 30 seconds everyone invited has "Budget" on 4 November 2026 from 14:00 to 15:00

    When Alice moves "Budget" to 5 November 2026 from 10:00 to 11:00
    Then within 30 seconds everyone invited has "Budget" on 5 November 2026 from 10:00 to 11:00
    And nobody has "Budget" on 4 November 2026
    And the history of "Budget" reads the same for everyone invited:
      | revision | status    | date            | from  | to    |
      | 1        | confirmed | 4 November 2026 | 14:00 | 15:00 |
      | 2        | confirmed | 5 November 2026 | 10:00 | 11:00 |
    # Bob answered before the move. His answer stands, marked as given to the
    # earlier time.
    And everyone invited sees Bob's answer to "Budget" as "yes", given to revision 1

    # Only the organizer's Domain says what a meeting is.
    When Bob tries to move "Budget" to 6 November 2026 from 09:00 to 10:00
    Then Bob's change is refused because only Alice can change "Budget"
    When Erin tries to cancel "Budget"
    Then Erin's change is refused because only Alice can change "Budget"
    And after 30 seconds everyone invited has "Budget" on 5 November 2026 from 10:00 to 11:00, at revision 2

    When Alice cancels "Budget"
    Then within 30 seconds everyone invited has "Budget" on 5 November 2026 shown as cancelled
    And the history of "Budget" reads the same for everyone invited:
      | revision | status    | date            | from  | to    |
      | 1        | confirmed | 4 November 2026 | 14:00 | 15:00 |
      | 2        | confirmed | 5 November 2026 | 10:00 | 11:00 |
      | 3        | cancelled | 5 November 2026 | 10:00 | 11:00 |
    And no email has left any Domain containing:
      | Budget |

  @rfc-15.4
  Scenario: Whoever is invited later starts at the invitation, whoever is removed gets nothing new
    Given Alice invited Bob and Erin to "Roadmap" on 10 November 2026 from 13:00 to 14:00 Europe/Amsterdam
    And Alice moved "Roadmap" to 10 November 2026 from 15:00 to 16:00
    And Bob answered "yes" to "Roadmap"
    And Erin answered "no" to "Roadmap"

    # Carol gets the meeting as it is now, and the answers given so far, each
    # from the Domain of the person who gave it.
    When Alice invites Carol to "Roadmap"
    Then within 30 seconds Carol has "Roadmap" on 10 November 2026 from 15:00 to 16:00, organized by Alice
    And Carol sees these answers to "Roadmap":
      | attendee | answer    |
      | Alice    | yes       |
      | Bob      | yes       |
      | Erin     | no        |
      | Carol    | no answer |
    And the history of "Roadmap" for Carol starts at revision 3
    And within 30 seconds everyone invited sees Alice, Bob, Erin and Carol invited to "Roadmap"

    # Only the organizer invites and removes.
    When Erin tries to invite Eddie to "Roadmap"
    Then Erin's change is refused because only Alice can change "Roadmap"
    And Eddie has no meetings

    When Alice removes Bob from "Roadmap"
    Then within 30 seconds Alice, Erin and Carol see Alice, Erin and Carol invited to "Roadmap"
    And Bob has "Roadmap" shown as cancelled for Bob
    When Alice moves "Roadmap" to 11 November 2026 from 15:00 to 16:00
    Then within 30 seconds Alice, Erin and Carol have "Roadmap" on 11 November 2026 from 15:00 to 16:00
    And Bob still has "Roadmap" on 10 November 2026 from 15:00 to 16:00, shown as cancelled for Bob

    # Bob's Domain no longer lists Bob as invited, so the answer goes nowhere.
    When Bob answers "maybe" to "Roadmap"
    Then after 30 seconds Alice, Erin and Carol see these answers to "Roadmap":
      | attendee | answer    |
      | Alice    | yes       |
      | Erin     | no        |
      | Carol    | no answer |
    And no email has left any Domain containing:
      | Roadmap |

  @rfc-15.6
  Scenario: A weekly meeting keeps its local time across a daylight saving change, with one week moved and one cancelled
    # Daylight saving time ends in Europe/Amsterdam on 25 October 2026, so the
    # same 09:00 is 07:00 UTC before and 08:00 UTC after.
    When Alice invites Bob and Erin to "Standup" every Tuesday from 20 October 2026, 4 times, from 09:00 to 09:15 Europe/Amsterdam
    Then within 30 seconds everyone invited has these occurrences of "Standup":
      | date             | from  | to    | from UTC | status    |
      | 20 October 2026  | 09:00 | 09:15 | 07:00    | confirmed |
      | 27 October 2026  | 09:00 | 09:15 | 08:00    | confirmed |
      | 3 November 2026  | 09:00 | 09:15 | 08:00    | confirmed |
      | 10 November 2026 | 09:00 | 09:15 | 08:00    | confirmed |

    When Alice moves only the occurrence of "Standup" on 27 October 2026 to 28 October 2026 from 09:00 to 09:15
    And Alice cancels only the occurrence of "Standup" on 3 November 2026
    Then within 30 seconds everyone invited has these occurrences of "Standup":
      | date             | from  | to    | from UTC | status    |
      | 20 October 2026  | 09:00 | 09:15 | 07:00    | confirmed |
      | 28 October 2026  | 09:00 | 09:15 | 08:00    | confirmed |
      | 3 November 2026  | 09:00 | 09:15 | 08:00    | cancelled |
      | 10 November 2026 | 09:00 | 09:15 | 08:00    | confirmed |
    And nobody has "Standup" on 27 October 2026

    # An answer can be for the whole series or for one occurrence, and the
    # later of the two counts for that occurrence.
    When Bob answers "yes" to every occurrence of "Standup"
    And Bob answers "no" to the occurrence of "Standup" on 10 November 2026
    Then within 30 seconds everyone invited sees Bob's answers to "Standup" as:
      | date             | answer |
      | 20 October 2026  | yes    |
      | 28 October 2026  | yes    |
      | 10 November 2026 | no     |
    When Bob answers "maybe" to every occurrence of "Standup"
    Then within 30 seconds everyone invited sees Bob's answers to "Standup" as:
      | date             | answer |
      | 20 October 2026  | maybe  |
      | 28 October 2026  | maybe  |
      | 10 November 2026 | maybe  |
    And no email has left any Domain containing:
      | Standup |

  @rfc-15.1 @rfc-15.4
  Scenario: Whoever cannot be reached is named to the organizer at once, and never listed
    Given Pete has an account on acme.test
    And acme.test does not implement calendar
    And Dave has no account on any Domain
    And nobody called "nobody@example.com" has an account on example.com

    When Alice invites Ann, Bob, Pete, "dave@gmail.test" and "nobody@example.com" to "Offsite" on 12 November 2026 from 09:00 to 17:00 Europe/Amsterdam
    # acme.test does not advertise calendar, and an address with no federated
    # home could only be reached by email, so both are refused before sending.
    Then Alice has a message from "mailer-daemon@103mail.com" naming "Offsite" and Pete and "dave@gmail.test" as unreachable
    And within 30 seconds Ann and Bob have "Offsite" on 12 November 2026 from 09:00 to 17:00, organized by Alice

    # example.com does not know the address, so Alice is told, and a new
    # revision takes it off the meeting for everyone.
    And within 30 seconds Alice has "Invitation failed: Offsite" naming "nobody@example.com" and the reason "unknown_recipient"
    And within 30 seconds everyone invited sees Alice, Ann and Bob invited to "Offsite"
    And Pete has no meetings
    And the bounces exist only on 103mail.com: Ann and Bob never see them
    And no email has left any Domain containing:
      | Offsite |
    And no email went to "dave@gmail.test"
    And acme.test received nothing from 103mail.com

  @rfc-15.7
  Scenario: Deleting a meeting hides it for one person only, until the next change
    Given Alice invited Ann, Bob and Carol to "Retro" on 13 November 2026 from 16:00 to 17:00 Europe/Amsterdam
    And Bob answered "yes" to "Retro"

    # Deleting is not declining: Bob stays invited and the answer stands.
    When Bob deletes "Retro"
    Then Bob has no meetings
    And after 30 seconds Alice, Ann and Carol still see Bob invited to "Retro" with the answer "yes"

    When Alice moves "Retro" to 13 November 2026 from 15:00 to 16:00
    Then within 30 seconds Bob has "Retro" on 13 November 2026 from 15:00 to 16:00, organized by Alice
    And Bob sees "Retro" as changed since Bob last looked at it
    And Carol has "Retro" exactly once

  @outage @rfc-15.3
  Scenario: A Domain that was down catches up on the latest revision, with the whole history in order
    Given Alice invited Ann, Bob and Erin to "Planning" on 17 November 2026 from 10:00 to 11:00 Europe/Amsterdam
    And within 30 seconds everyone invited has "Planning" on 17 November 2026 from 10:00 to 11:00

    When example.com goes down
    And Alice moves "Planning" to 17 November 2026 from 11:00 to 12:00
    And Alice moves "Planning" to 18 November 2026 from 11:00 to 12:00
    And Erin answers "yes" to "Planning"
    Then every change is confirmed within 2 seconds
    And within 30 seconds Ann and Erin have "Planning" on 18 November 2026 from 11:00 to 12:00

    When example.com comes back
    Then within 3 minutes Bob has "Planning" on 18 November 2026 from 11:00 to 12:00
    # Revision 2 may reach example.com after revision 3. It is stored in its
    # place and does not move the meeting back.
    And the history of "Planning" reads the same for everyone invited:
      | revision | status    | date             | from  | to    |
      | 1        | confirmed | 17 November 2026 | 10:00 | 11:00 |
      | 2        | confirmed | 17 November 2026 | 11:00 | 12:00 |
      | 3        | confirmed | 18 November 2026 | 11:00 | 12:00 |
    And Bob sees Erin's answer to "Planning" as "yes"
    And nobody has "Planning" twice

  @protocol @rfc-15.3 @rfc-15.5
  Scenario: Another Domain can neither change a meeting nor answer for someone else
    Given the test peer is known to 103mail.com and example.com
    And Alice invited Ann, Bob and Carol to "Board" on 19 November 2026 from 09:00 to 10:00 Europe/Amsterdam
    And within 30 seconds everyone invited has "Board" on 19 November 2026 from 09:00 to 10:00

    # The answer to an answer from someone not invited depends on the revision
    # it claims: one the Destination holds means they were removed (202,
    # nothing changes), a later one may still be on its way (425, retried).
    When the test peer tries each of these against example.com, aimed at the "Board" meeting:
      | attempt                                                                | answer | error              |
      | revision 9 of "Board" with organizer "alice@103mail.com"               | 400    | schema_violation   |
      | revision 9 of "Board" with organizer "mallory@peer.test"               | 400    | organizer_mismatch |
      | an answer "no" for "bob@example.com"                                   | 400    | schema_violation   |
      | an answer "yes" for "mallory@peer.test", not invited, to revision 1    | 202    | none               |
      | an answer "yes" for "mallory@peer.test", not invited, to revision 2    | 425    | not_a_member       |
      | an envelope of kind "poll"                                             | 400    | unsupported_kind   |
    And the test peer tries each of these against example.com, inviting Bob to a new meeting:
      | attempt                                                                | answer | error              |
      | a weekly meeting whose repeat has the unknown member "by_set_position" | 400    | unsupported_repeat |
      | a meeting in the time zone "Mars/Olympus_Mons"                         | 400    | unknown_time_zone  |
      | an all-day meeting starting at 09:00                                   | 400    | schema_violation   |
    Then each attempt gets the answer and error listed
    And Bob has only the meeting "Board"
    And after 30 seconds everyone invited has "Board" on 19 November 2026 from 09:00 to 10:00, at revision 1
    And everyone invited sees these answers to "Board":
      | attendee | answer    |
      | Alice    | yes       |
      | Ann      | no answer |
      | Bob      | no answer |
      | Carol    | no answer |
