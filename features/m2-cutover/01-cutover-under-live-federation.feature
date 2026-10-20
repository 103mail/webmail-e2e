@M2 @wip @outage
Feature: 103mail.com moves to the new backend and database while peers keep sending
  As people on 103mail.com and on its peers
  We want the switch from the old backend to the new one to be invisible
  Even for conversations that are in flight during the switch

  Background:
    Given 103mail.com runs on the old backend on MySQL with a copy of production data
    And the new backend runs on a database migrated from that copy
    And Alice and Ann are existing 103mail.com users with mail, attachments and unread threads
    And Bob and Carol have accounts on example.com
    And Erin has an account on mail.acme.test

  Scenario: Existing mail, then a switch in the middle of a conversation
    Given Alice and Ann's folders, threads, messages, attachments and unread state are recorded from the old backend
    And Alice wrote to Ann, Bob and Erin: "Before the switch"

    When example.com and mail.acme.test keep sending to Alice and Ann during the switch
    And 103mail.com switches to the new backend
    Then every table holds the same number of rows with the same checksums as the copy it was migrated from
    And Alice and Ann sign in on the new backend with their existing passwords
    And their folders, threads, messages, attachments and unread state are identical to the recording
    And within 3 minutes every message sent to them during the switch is in their Inbox exactly once

    When Bob replies to "Before the switch": "Did you move?"
    And Ann replies to "Before the switch": "We did"
    Then within 30 seconds the thread "Before the switch" reads the same for Alice, Ann, Bob and Erin

  Scenario: Rolling back loses nothing written before the switch
    Given 103mail.com has switched to the new backend and the old backend is read-only
    When 103mail.com switches back to the old backend
    Then every thread Alice and Ann had before the switch is still there
    And the rollback is reported as losing only mail received after the switch, listed by thread
