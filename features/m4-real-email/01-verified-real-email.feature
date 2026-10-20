@M4 @wip
Feature: Everyone on every Domain has a verified real email address
  As people joining Domains across the federation
  We want to prove an address outside the system is ours before we can use an account
  So that notifications reach the right person and Registrars can find us by it later

  Background:
    Given Alice has an account on 103mail.com
    And Bob has an account on example.com

  Scenario: Several people sign up on several Domains, one of them twice
    When these people sign up:
      | person | Domain         | real email          |
      | Frank  | 103mail.com    | frank@gmail.test    |
      | Grace  | 103mail.com    | grace@gmail.test    |
      | Hank   | example.com    | hank@gmail.test     |
      | Ivy    | mail.acme.test | ivy@gmail.test      |
      | Jack   | 103mail.com    | frank@gmail.test    |
      | Kim    | example.com    |                     |
    Then Jack is refused with "This email address is already in use"
    And Kim is refused with "A real email address is required"
    And a confirmation email went to Frank, Grace, Hank and Ivy at their real addresses
    And none of Frank, Grace, Hank or Ivy can sign in yet

    When Frank, Hank and Ivy follow the link in their confirmation email
    Then Frank, Hank and Ivy can sign in
    And Grace still cannot
    When anyone follows Frank's confirmation link again
    Then they are told the link has already been used

    When Alice writes to Frank, Grace, Hank and Ivy: "Welcome aboard"
    Then within 2 minutes Frank, Hank and Ivy each have "Welcome aboard" in their Inbox
    And a notification went to "frank@gmail.test", "hank@gmail.test" and "ivy@gmail.test"
    And no notification contains "Welcome aboard"
    And Grace's message waits until she confirms, with no notification to an unconfirmed address

  Scenario: The same real email on two Domains is allowed
    When Lena signs up on 103mail.com with the real email "lena@gmail.test"
    And Lena signs up on example.com with the real email "lena@gmail.test"
    Then both sign-ups succeed after confirmation
    And Bob can write to either of Lena's accounts and each gets only its own mail
