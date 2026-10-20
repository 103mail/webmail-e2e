@M1 @rfc-7
Feature: Files travel intact between Domains
  As people sharing files across Domains
  We want every recipient to get exactly the bytes the sender attached
  So that a file can be trusted wherever it is opened

  Background:
    Given Alice and Ann have accounts on 103mail.com
    And Bob and Carol have accounts on example.com
    And Erin and Eddie have accounts on mail.acme.test

  Scenario: A document set, a revision, and replies with files from every Domain
    When Alice writes to Ann, Bob and Erin: "Tender documents" attaching "tender.pdf" (8 MB), "pricing.xlsx" (400 KB) and "logo.png" (120 KB)
    Then within 60 seconds Ann, Bob and Erin each see 3 attachments on "Tender documents"
    And every recipient's copy of each file is byte-identical to Alice's

    When Bob replies to "Tender documents" attaching "pricing-v2.xlsx" (410 KB)
    And Erin replies to "Tender documents" attaching "signed.pdf" (2 MB)
    Then within 60 seconds Alice and Ann each see "pricing-v2.xlsx" and "signed.pdf"
    And Bob sees "signed.pdf" and Erin sees "pricing-v2.xlsx", each byte-identical

    When Carol is invited to "Tender documents"
    Then within 30 seconds Carol can download all 5 files, each byte-identical

    And no email has left any Domain with an attachment

  Scenario: The same file sent twice is stored once per Domain
    When Alice writes to Bob and Carol: "Logo" attaching "logo.png" (120 KB)
    And Ann writes to Bob: "Logo again" attaching the same "logo.png"
    Then within 30 seconds Bob sees "logo.png" on both threads
    And example.com stores the bytes of "logo.png" once

  Scenario: Limits are enforced on both sides of the wire
    When Alice writes to Bob and Erin: "Video" attaching "raw.mov" (30 MB)
    Then Alice is told the attachment is larger than the 25 MB limit
    And nothing is delivered to example.com or mail.acme.test

    Given example.com's attachment limit is 5 MB
    When Alice writes to Bob and Erin: "Scan" attaching "scan.pdf" (8 MB)
    Then within 30 seconds Erin has "Scan" with "scan.pdf"
    And Alice has the thread "Delivery failed: Scan" naming "bob@example.com" and the reason "too_large"
    And Bob has no thread "Scan"

  @protocol
  Scenario: A tampered file is refused and the honest copy still arrives
    Given the test peer is known to example.com
    And Alice wrote to Bob and Carol: "Contract" attaching "contract.pdf" (1 MB)
    When the test peer sends Bob and Carol a message whose attachment does not match its declared hash
    Then example.com answers 400 "attachment_mismatch"
    And Bob and Carol each have exactly one thread with an attachment: Alice's "Contract"
