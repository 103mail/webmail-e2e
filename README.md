# webmail-e2e

End-to-end acceptance tests for federated webmail. Every promise the product
makes is written here as a story in plain English, and every story runs against
several real Domains with several people on each, through the public API only.
A milestone in `../PLANNING.md` is done when its stories are green, and not
before.

The stories are the specification. If a story and the RFC disagree, one of them
is wrong and gets fixed in the same change.

## Rules of this suite

1. **API only.** Every step acts through the public JSON API as a signed-in
   person, or through the federation endpoints as a peer. No browser, no UI
   tests, no reading the database to decide an outcome.
2. **No simple tests.** A story involves at least two federated Domains and at
   least two people on each. Single-endpoint checks belong in the unit and
   integration tests of the repo that owns the code, not here.
3. **A story is a journey.** It runs in acts: something happens, then the
   outcome for every person involved is checked, then the next thing happens.
   A story ends only when the world has settled and every person's view agrees.
4. **Outcomes are exact and per person.** Who sees which thread, in which
   folder, with how many messages, in which order, read or unread. "It works"
   is not an outcome.
5. **Every story checks what must not happen.** No message content left by
   email, no account on the wrong Domain, no duplicate, no stray message in
   someone's Inbox.
6. **Time is explicit.** Asynchronous outcomes carry a deadline ("within 30
   seconds"). Steps poll until the deadline; nothing sleeps.
7. **Stories stand alone.** Each one creates fresh accounts with unique
   addresses, so stories run in any order and in parallel.

## How we work: story first, red, green

1. **Write the story** and have a person review it before any code is written.
2. **Commit it tagged `@wip`.** It must fail, and fail because the behaviour is
   missing, not because the harness is broken.
3. **Build inward** with unit and integration tests in the owning repo. The
   story stays red until the whole stack behaves.
4. **Go green and drop `@wip`.** CI runs every untagged story on every change to
   any repo. A red untagged story blocks the merge.
5. **Never weaken a story to make it pass.** Changing an expected outcome is a
   product decision and is reviewed like one.

## The world

The same Domains and people appear in every story, so the stories read like
episodes of one series. Each name resolves to a fresh account per story.

| Domain | Kind | People |
| --- | --- | --- |
| `103mail.com` | Our Domain; becomes a Registrar in M5 | Alice, Ann, Arjen |
| `example.com` | Independent peer | Bob, Carol, Chris |
| `mail.acme.test` | Peer on a subdomain | Erin, Eddie |
| `acme.test` | Separate Domain, parent of the one above | Pete |
| `registrar.test` | Second Registrar (M6) | Rita |
| `peer.test` | Scripted peer the tests control; misbehaves on purpose | the test peer |
| `gmail.test` | Ordinary email, no federation | Dave, Dana |

## Step vocabulary

Stories reuse a small set of phrases, so they stay readable and the step
definitions stay few. The main ones:

| Phrase | Means |
| --- | --- |
| `Alice writes to Bob, Carol and Erin: "Subject"` | Starts a thread in Alice's Inbox folder with those subscribers |
| `... with the body "..."` / `... attaching "file" (2 MB)` | Body and attachments of that message |
| `Bob replies to "Subject": "..."` | Appends a message to the thread as Bob |
| `Carol invites Chris to "Subject"` | Adds a subscriber to an existing thread |
| `within 30 seconds everyone sees:` + table | Polls each person's API view until the table matches |
| `the thread "Subject" reads the same for everyone` | Same messages, same order, same senders, for every member |
| `no email has left any Domain containing:` + list | Checked against the mail sink of every Domain |
| `<domain> holds accounts only for its own people` | No account exists for an address on another Domain |
| `<domain> goes down` / `comes back` | The test stack stops or restarts that Domain's API and worker |
| `the test peer sends ...` | A raw signed (or deliberately mis-signed) federation request |

## Layout and tags

```
features/
  m1-basic/            federation between Domains, Basic tier of the RFC
  m2-cutover/          103mail.com moves to the Go backend under live federation
  m3-live/             staging and production against an independent peer
  m4-real-email/       verified real email addresses on every Domain
  m5-registrar/        reaching people with no federated home
  m6-multi-registrar/  several Registrars agreeing on one mailbox per person
```

| Tag | Meaning |
| --- | --- |
| `@M1` ... `@M6` | Milestone whose exit criterion includes this story |
| `@protocol` | Part of the story acts as a raw federation peer |
| `@outage` | Stops or restarts a Domain, or shortens the retry budget |
| `@wip` | Agreed but not built yet; allowed to fail |
| `@rfc-x.y` | The RFC section this story proves |

## The test stack

The compose stack in `../webmail-server/deploy` grows to what the stories need:

- **Domains**: 103mail.com, example.com, mail.acme.test and acme.test, each an
  API, a worker and an SQLite file in a volume. registrar.test joins for M6.
  There are no database containers: the whole suite runs on SQLite, while
  production runs PostgreSQL on the same portable SQL (ADR 19 in
  `../webmail-server/docs/adr/`).
- **The test peer** (`peer.test`): a Domain like the others in the stack,
  publishing a real descriptor from its own key, that nobody uses. The steps
  sign envelopes with its key (`deploy/keys/peer.test.ed25519.pem`) and send
  them as it: stale, replayed, mis-signed, oversized, forged, or from someone
  who is not on the thread (`peer.go`). A step can speak as any Domain of the
  stack the same way, with that Domain's key, to try to speak for another.
- **The mail sink**: outbound email from every Domain is captured instead of
  sent, so stories can read invitations and prove that no content left. It
  stands in for SNS, so each server publishes through its production path,
  and the steps read it at `http://127.0.0.1:8025/messages` (or
  `WEBMAIL_E2E_MAILSINK`). Email checks see only what was sent during the
  story.
- **Controls**: steps can take a Domain down and bring it back, move a Domain's
  clock forward (the server takes the time from Go, never from the database),
  and set test values for the retry budget (2 minutes instead of 72 hours), the
  descriptor cache and the rate limit. A story that relies on one says so in a `Given`.
  Taking a Domain down and the retry budget are built (`outage.go`): they stop
  and start the containers labelled with the Domain, and recreate its worker
  with a shorter budget. Whatever a story changes is put back after it, even
  when it fails. The clock is built too, as one clock for the whole stack: see
  "Time" below. The descriptor cache is not built yet.
  A story can also change a Domain's federation config for its duration,
  through `FEDERATION_CONFIG_<DOMAIN>`: an allowlist, a smaller attachment
  limit, a lower inbound rate limit (`ratelimit.go`), or a rotated key with
  the old one published as previous (`reconfigure.go`). And every federation request passes through a fault
  proxy, `deploy/faults`, which a story can arm at `http://127.0.0.1:8026` (or
  `WEBMAIL_E2E_FAULTS`) to lose an answer or to refuse with a given status.
  Both are put back after the story.

## Time

A deadline such as "within 3 minutes" is not waited out. The stack's clock is
the real one plus an offset every Domain reads (`deploy/clock/offset`), and
while a step's check fails, the step moves that offset forward a few seconds
at a time and checks again a quarter of a second later. Retries, backoffs, the
retry budget and the backfill probe all run on that clock, so minutes of them
pass in seconds. Once the deadline has passed in virtual time, the step keeps
checking for up to 20 real seconds, the time the workers need to act on the
last move, and then fails. Only things no clock speeds up are waited for in
real time, such as a container starting. The offset only grows: moving it back
would put new messages before old ones.

## Runner

Stories run with [godog](https://github.com/cucumber/godog), Cucumber for Go.
The suite drives HTTP only, so there is no browser to install, and Go matches
the backend: signing a federation request in a step uses the same
`crypto/ed25519` the server does.

    go test ./...                                   # every story not tagged @wip
    go test ./... -godog.tags='@M1 && ~@wip'        # one milestone
    go test ./... -godog.tags='@wip'                # what is still to build
    go test ./... -godog.tags= -godog.paths=features/m1-basic/01-project-thread.feature

The suite runs against the compose stack in `../webmail-server/deploy`, which
must be up (see its `TESTING.md`); set `WEBMAIL_E2E_DEPLOY` to use another
checkout. A Domain a story needs but the stack lacks fails at the `Given` that
creates its people, with a message naming the Domain.

The step definitions live in `steps.go`, with the per-story state in
`world.go` and the API client in `client.go`. Steps whose phrase is agreed but
whose stack control does not exist yet answer pending with the reason.

What a Domain sent to its peers, and which accounts it holds, cannot be seen
through the public API. Those steps read the read-only controls each Domain's
API serves at `http://webmail.{domain}:8081/control/` (ADR 20 in
`../webmail-server/docs/adr/`), and only look at envelopes and accounts that
carry the story's own addresses.

Until there is a signup API (M4), `Stack.Provision` creates each person's
account by writing the same rows `deploy/seed.sh` writes into the Domain's
SQLite file. That is a stack control, like taking a Domain down; no step reads
a database to decide an outcome.

## RFC coverage

Every MUST in the Basic tier of `../webmail-federation/project_rfc.md` is proven
by at least one story tagged with its section. A section with no tag is a gap
in the suite.

| RFC section | Proven by |
| --- | --- |
| 4, 4.3 Discovery and trust | `m1-basic/07-key-rotation-mid-conversation`, `m1-basic/09-subdomains-and-parents` |
| 4.1.1 Subdomain Domains | `m1-basic/09-subdomains-and-parents` |
| 5, 5.4 Envelope and idempotency | `m1-basic/01-project-thread`, `m1-basic/06-hostile-peer` |
| 5.5 Message order | `m1-basic/01-project-thread` |
| 5.6 Membership changes | `m1-basic/02-membership-across-domains`, `m1-basic/05-mixed-delivery-and-bounces` |
| 6.2 Signature | `m1-basic/06-hostile-peer` |
| 6.3, 6.4 Responses and retry | `m1-basic/04-outages`, `m1-basic/08-allowlists-and-rate-limits` |
| 6.5 Bounces | `m1-basic/05-mixed-delivery-and-bounces` |
| 7 Attachments | `m1-basic/03-attachments` |
| 8 Registrar | `m5-registrar/*`, `m6-multi-registrar/*` |
| 9.5 Key rotation | `m1-basic/07-key-rotation-mid-conversation` |
| 9.7, 9.10 Allowlist, rate limiting | `m1-basic/08-allowlists-and-rate-limits` |
