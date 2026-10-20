@M3 @wip
Feature: 103mail.com federates with a Domain someone else operates
  As 103mail.com and the operator of an independent Domain
  We want a full conversation between our users over the public internet
  So that federation is proven against an implementation we do not control

  This story runs in staging and then in production, against the partner
  Domain named in the run configuration. Accounts on the partner Domain are
  provided by its operator.

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Paula and Piet have accounts on the partner Domain
    And 103mail.com accepts mail only from the partner Domain

  Scenario: A full conversation over the public internet
    When Alice writes to Ann, Paula and Piet: "Hello from 103mail" attaching "intro.pdf" (1 MB)
    Then within 2 minutes Paula and Piet have "Hello from 103mail" with "intro.pdf", byte-identical
    When Paula replies to "Hello from 103mail": "Hello back"
    And Piet invites another partner colleague, Quinn, to "Hello from 103mail"
    And Quinn replies to "Hello from 103mail": "Joining late"
    Then within 2 minutes the thread reads the same on both Domains with 3 messages
    And everyone sees Alice, Ann, Paula, Piet and Quinn as members
    And 103mail.com holds accounts only for its own people
    And no email has left 103mail.com containing "Hello from 103mail" or "Hello back"
    And https://webmail.103mail.com/.well-known/webmail-federation is served with a certificate valid for that exact name
