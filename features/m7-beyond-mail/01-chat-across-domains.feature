@M7 @wip @rfc-14
Feature: People chat with each other across Domains
  As people with accounts on different Domains
  We want a chat to reach everyone in it, wherever their account is
  So that chat works between Domains the way mail does, and never leaves the system

  # Chat is not federated yet: a chat with someone on another Domain gives
  # them no chat at all. These stories prove the chat extension, §14 of the
  # RFC, and stay @wip until it is built. Within a Domain, the rules are those
  # of ADR 9 in ../webmail-server/docs/adr/.

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  @rfc-14.2 @rfc-14.3 @rfc-14.7
  Scenario: A direct chat reaches the other Domain and reads the same on both sides
    When Alice starts a chat with Bob: "Lunch tomorrow?"
    Then within 30 seconds Bob has a chat with Alice whose last message is "Lunch tomorrow?"
    And Bob has 1 unread message in the chat with Alice

    When Bob writes in the chat with Alice: "Sure, 12:30"
    Then within 30 seconds the chat between Alice and Bob reads the same for both:
      | from  | message         |
      | Alice | Lunch tomorrow? |
      | Bob   | Sure, 12:30     |
    And Ann, Carol, Erin and Eddie have no chats
    And no email has left any Domain containing:
      | Lunch tomorrow? |
      | Sure, 12:30     |
    And 103mail.com holds accounts only for its own people
    And example.com holds accounts only for its own people

  @rfc-14.3 @rfc-14.4
  Scenario: A group chat spans three Domains and every member sees the same
    Given Alice started the group chat "Launch" with Ann, Bob and Erin
    Then within 30 seconds Ann, Bob and Erin each have the group chat "Launch" with Alice, Ann, Bob and Erin as members

    When Erin writes in "Launch": "Docs are ready"
    And Bob writes in "Launch": "Shipping Monday"
    # A group reaches the other Domains with a message saying it was created.
    Then within 30 seconds the chat "Launch" reads the same for everyone:
      | from  | message                        |
      | Alice | Alice created the group Launch |
      | Erin  | Docs are ready                 |
      | Bob   | Shipping Monday                |
    And Carol and Eddie have no chats
    And no email has left any Domain containing:
      | Docs are ready  |
      | Shipping Monday |
    And mail.acme.test holds accounts only for its own people

  @rfc-14.5
  Scenario: Whoever joins later sees the history, whoever leaves gets nothing new
    Given Alice started the group chat "Launch" with Bob and Erin
    And Bob wrote in "Launch": "Draft is in the drive"

    # A new participant sees all prior history (ADR 9), sent to Carol's Domain
    # by each Domain for its own users' messages.
    When Alice adds Carol to "Launch"
    Then within 30 seconds Carol has the group chat "Launch" with Alice, Bob, Erin and Carol as members
    And Carol sees "Draft is in the drive" in "Launch"

    # Someone who left keeps what was sent before and receives nothing after.
    When Bob leaves the chat "Launch"
    Then within 30 seconds every member of the chat "Launch" sees Alice, Erin and Carol as members
    When Erin writes in "Launch": "Bob is gone"
    Then within 30 seconds Alice and Carol see "Bob is gone" in "Launch"
    And Bob never sees "Bob is gone" in "Launch"
    And Bob still sees "Draft is in the drive" in "Launch"

    When Bob tries to write in "Launch": "Wait, I'm back"
    Then Bob's message is refused because he is no longer in the chat
    And nobody sees "Wait, I'm back" in "Launch"

  @rfc-14.6
  Scenario: Deleting a chat hides it on one Domain only, until the next message
    Given Alice started a chat with Bob: "Quick question"

    # Deleting is not leaving: Bob stays a participant on every Domain.
    When Bob deletes the chat with Alice
    Then Bob has no chat with Alice
    And Alice still has the chat with Bob, with Alice and Bob as members

    When Alice writes in the chat with Bob: "Still there?"
    Then within 30 seconds Bob has a chat with Alice whose last message is "Still there?"
    And the chat between Alice and Bob reads the same for both:
      | from  | message        |
      | Alice | Quick question |
      | Alice | Still there?   |
    And Ann and Carol have no chats

  @rfc-14.7
  Scenario: The sender sees who has read their message, on every Domain
    Given Alice started the group chat "Launch" with Ann, Bob and Erin
    And Alice wrote in "Launch": "Can everyone review the draft?"
    Then within 30 seconds Alice sees her message "Can everyone review the draft?" delivered to 3 and read by 0 of the 3 others

    # Ann is on Alice's own Domain; Bob and Erin are not. Each read reaches
    # Alice's Domain all the same.
    When Bob opens the chat "Launch"
    Then within 30 seconds Alice sees her message "Can everyone review the draft?" delivered to 3 and read by 1 of the 3 others
    When Ann and Erin open the chat "Launch"
    Then within 30 seconds Alice sees her message "Can everyone review the draft?" delivered to 3 and read by 3 of the 3 others
