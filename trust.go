package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerTrust binds the phrases of the stories about who a Domain trusts:
// a hostile peer (06), key rotation (07), allowlists (08) and subdomains
// (09), and the stack controls they need, which are put back after every
// story.
func (w *world) registerTrust(sc *godog.ScenarioContext) {
	// The test peer and Domains speaking as one another
	sc.Step(`^the test peer is known to (.+)$`, w.peerKnown)
	sc.Step(`^the test peer tries each of these against (\S+), aimed at the "([^"]*)" thread:$`, w.peerTries)
	sc.Step(`^each attempt gets the answer and error listed$`, w.attemptsAnswered)
	sc.Step(`^the test peer sends (.+) a message whose attachment does not match its declared hash$`, w.peerTampers)
	sc.Step(`^the test peer replays (\S+)'s last envelope to (\S+) with a fresh signature of its own$`, w.peerReplays)
	sc.Step(`^the test peer sends (\S+) a reply to "([^"]*)" from "([^"]*)"$`, w.peerIntrudes)
	sc.Step(`^(\S+) sends (\S+) a reply to "([^"]*)" with the sender "([^"]*)"$`, w.domainImpersonates)
	sc.Step(`^(\S+) answers (\d+) "([^"]*)"$`, w.answers)
	sc.Step(`^(\S+) refuses to add it to "([^"]*)"$`, w.refusesToAdd)

	// Faults on the wire
	sc.Step(`^(\S+) accepts the next envelope from (\S+) but the answer is lost$`, w.answerLost)
	sc.Step(`^(\S+) will refuse (\S+)'s next envelope with (\d+) "([^"]*)"$`, w.willRefuse)
	sc.Step(`^(\S+) retries the delivery$`, w.retriesDelivery)
	sc.Step(`^(\S+) makes no further delivery attempt for that envelope$`,
		func(ctx context.Context, domain string) error { return w.noFurtherAttempt(ctx, domain, "") })

	// Configuration
	sc.Step(`^(\S+) rotates its signing key$`, w.rotates)
	sc.Step(`^(.+) now pin (\S+)'s new key$`, w.pinNewKey)
	sc.Step(`^(\S+) only accepts mail from (.+)$`, w.onlyAccepts)

	// Outcomes
	sc.Step(`^within (\d+) seconds everyone has "([^"]*)", so every Domain has pinned the others' keys$`, w.everyoneHasPinned)
	sc.Step(`^within (\d+) seconds (.+?) have "([^"]*)"$`, w.haveThread)
	sc.Step(`^after (\d+) seconds everyone still sees exactly (\d+) messages on "([^"]*)"$`, w.stillExactly)
	sc.Step(`^the member list of "([^"]*)" is unchanged for everyone$`, w.membersUnchanged)
	sc.Step(`^within (\d+) (seconds?|minutes?) (.+?) each see "([^"]*)" exactly once$`,
		func(ctx context.Context, n int, unit, list, body string) error {
			return eventually(ctx, toSeconds(n, unit), func() error { return w.seeOnce(ctx, list, body) })
		})
	sc.Step(`^(.+?) still see "([^"]*)" exactly once$`, w.seeOnce)
	sc.Step(`^nobody sees a message from "([^"]*)"$`, w.nobodyHearsFrom)
	sc.Step(`^"([^"]*)" is not a member of "([^"]*)" for anyone$`, w.notAMember)
	sc.Step(`^([\w ,]+?) still sees? "([^"]*)"$`, w.stillSeeBody)
	sc.Step(`^(\w+) still sees (\d+) messages? on "([^"]*)"$`, w.stillSeesCount)
	sc.Step(`^within (\d+) seconds (.+?) sees? (\w+)'s reply$`,
		func(ctx context.Context, seconds int, list, author string) error {
			return w.eventuallySee(ctx, seconds, list, w.replyBy[author])
		})
	sc.Step(`^(.+?) never sees? (\w+)'s reply$`,
		func(ctx context.Context, list, author string) error { return w.neverSee(ctx, list, w.replyBy[author]) })
	sc.Step(`^within (\d+) seconds (\w+) has "([^"]*)" naming (.+) and the reason "([^"]*)"$`, w.bounceNaming)
	sc.Step(`^(\w+) has (?:the thread )?"([^"]*)" naming (.+) and the reason "([^"]*)"$`,
		func(ctx context.Context, who, subject, list, reason string) error {
			return w.bounceNaming(ctx, 30, who, subject, list, reason)
		})
	sc.Step(`^(\S+) discovered (.+)$`, w.discovered)
	sc.Step(`^the two descriptors carry different keys$`, w.differentKeys)
	sc.Step(`^(\w+) has no "([^"]*)" until (\S+) comes back$`, w.noneUntilBack)

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		var errs []error
		if w.armed {
			errs = append(errs, w.stack.clearFaults(ctx, ""))
		}
		for _, domain := range w.reconfigured {
			errs = append(errs, w.stack.reconfigure(ctx, domain, nil))
		}
		for _, key := range w.newKeys {
			os.Remove(key)
		}
		w.armed, w.reconfigured, w.newKeys = false, nil, nil
		return ctx, errors.Join(errs...)
	})
}

// reconfigure changes a Domain's federation config for the rest of the story.
func (w *world) reconfigure(ctx context.Context, domain string, cfg map[string]any) error {
	if !slices.Contains(w.reconfigured, domain) {
		w.reconfigured = append(w.reconfigured, domain)
	}
	return w.stack.reconfigure(ctx, domain, cfg)
}

// ── The test peer ───────────────────────────────────────────────────────────

// testPeer is the scripted peer every story shares.
const testPeer = "peer.test"

// peerKnown checks that the test peer can speak, and that the Domains it
// will speak to are up. Any Domain in the stack can discover it: it is a
// Domain like the others, and trust in it is first use (RFC 4.3).
func (w *world) peerKnown(ctx context.Context, list string) error {
	if _, err := w.stack.sender(testPeer); err != nil {
		return err
	}
	var errs []error
	for _, domain := range names(list) {
		errs = append(errs, w.stack.Check(ctx, domain))
	}
	return errors.Join(errs...)
}

// threadAt finds a conversation's federation id on a Domain, from the
// thread its members there are on.
func (w *world) threadAt(ctx context.Context, domain string, c *conversation) (string, []string, error) {
	var local, all []string
	for _, p := range c.members {
		all = append(all, p.address)
		if p.domain == domain {
			local = append(local, p.address)
		}
	}
	if len(local) == 0 {
		return "", nil, fmt.Errorf("nobody on %q lives on %s", c.subject, domain)
	}
	threads, err := w.stack.threadsOf(ctx, domain, local[0])
	if err != nil {
		return "", nil, err
	}
	for _, t := range threads {
		if !slices.ContainsFunc(all, func(a string) bool { return !slices.Contains(t.Members, a) }) {
			return t.FederationID, local, nil
		}
	}
	return "", nil, fmt.Errorf("%s holds no federated thread with everyone on %q", domain, c.subject)
}

// attempt is one row of the test peer's table, and the answer it got.
type attempt struct {
	what, wantStatus, wantCode string
	got                        answer
}

var (
	claimingFrom = regexp.MustCompile(`claiming to come from (\S+)`)
	withSender   = regexp.MustCompile(`with the sender "([^"]+)"`)
	datedAgo     = regexp.MustCompile(`dated (\d+) minutes ago`)
	withVersion  = regexp.MustCompile(`protocol version "([^"]+)"`)
	addressedTo  = regexp.MustCompile(`addressed to (\d+) recipients`)
)

// peerTries makes each attempt in the table against a Domain, every one of
// them a reply on the thread from the test peer, wrong in the way the row
// says and right in every other way.
func (w *world) peerTries(ctx context.Context, destination, subject string, table *godog.Table) error {
	rows, err := tableRows(table)
	if err != nil {
		return err
	}
	if !slices.Equal(rows[0], []string{"attempt", "answer", "error"}) {
		return fmt.Errorf("unknown table %v; expected [attempt answer error]", rows[0])
	}
	peer, err := w.stack.sender(testPeer)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	threadID, recipients, err := w.threadAt(ctx, destination, c)
	if err != nil {
		return err
	}
	// verified are nonces of requests whose signature was accepted, which a
	// Domain remembers (RFC 9.4).
	var verified []string
	w.attempts = nil
	for _, row := range rows[1:] {
		what := row[0]
		e := peer.envelope("test@"+testPeer, threadID, subject, "Let me in", recipients)
		if m := claimingFrom.FindStringSubmatch(what); m != nil {
			e.OriginDomain = m[1]
			e.Message.SenderEmail = "someone@" + m[1]
		}
		if m := withSender.FindStringSubmatch(what); m != nil {
			e.Message.SenderEmail = w.rewrite(m[1])
		}
		if m := addressedTo.FindStringSubmatch(what); m != nil {
			n, _ := strconv.Atoi(m[1])
			e.Recipients = nil
			for i := range n {
				e.Recipients = append(e.Recipients, fmt.Sprintf("member%d.%s@%s", i, w.run, destination))
			}
		}
		e.Thread.Members = append([]string{e.Message.SenderEmail}, e.Recipients...)
		r, err := peer.request(e)
		if err != nil {
			return err
		}
		switch {
		case strings.Contains(what, "key it does not publish"):
			r.key = newKey()
		case strings.Contains(what, "changed after signing"):
			r.tamper = func(b []byte) []byte { return []byte(strings.Replace(string(b), "Let me in", "Let us in", 1)) }
		case datedAgo.MatchString(what):
			n, _ := strconv.Atoi(datedAgo.FindStringSubmatch(what)[1])
			r.date = time.Now().Add(-time.Duration(n) * time.Minute)
		case strings.Contains(what, "reusing a nonce"):
			if len(verified) == 0 {
				return fmt.Errorf("no earlier attempt had its signature accepted, so there is no nonce to reuse")
			}
			r.nonce = verified[0]
		case withVersion.MatchString(what):
			r.version = withVersion.FindStringSubmatch(what)[1]
		case strings.Contains(what, "envelope limit"):
			limit, err := w.envelopeLimit(ctx, destination)
			if err != nil {
				return err
			}
			r.body = append(r.body, []byte(strings.Repeat(" ", int(limit)+1-len(r.body)))...)
		case strings.Contains(what, "plain text"):
			r.contentType = "text/plain; charset=utf-8"
		case strings.Contains(what, "compressed body"):
			r.contentEncoding = "gzip"
		}
		got, err := peer.send(ctx, destination, r)
		if err != nil {
			return err
		}
		// A Domain checks the signature after the media type, size, version
		// and date, and claims the nonce once it verifies; what it refuses
		// after that, such as sender_mismatch, comes later (RFC 6.2, 9.4).
		if got.status != 413 && got.status != 415 && !slices.Contains(
			[]string{"unsupported_version", "stale_request", "bad_signature", "replay"}, got.code) {
			verified = append(verified, r.nonce)
		}
		w.attempts = append(w.attempts, attempt{what: what, wantStatus: row[1], wantCode: row[2], got: got})
	}
	return nil
}

// newKey is a key nobody publishes.
func newKey() ed25519.PrivateKey {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	return key
}

// envelopeLimit is the max_envelope_bytes a Domain advertises.
func (w *world) envelopeLimit(ctx context.Context, domain string) (int64, error) {
	d, err := w.stack.descriptor(ctx, domain)
	if err != nil {
		return 0, err
	}
	limit, ok := d["max_envelope_bytes"].(float64)
	if !ok || limit <= 0 {
		return 10 << 20, nil
	}
	return int64(limit), nil
}

func (w *world) attemptsAnswered() error {
	if len(w.attempts) == 0 {
		return fmt.Errorf("the test peer made no attempts")
	}
	var errs []error
	for _, a := range w.attempts {
		if strconv.Itoa(a.got.status) != a.wantStatus || a.got.code != a.wantCode {
			errs = append(errs, fmt.Errorf("%q was answered %s, want %s %q", a.what, a.got, a.wantStatus, a.wantCode))
		}
	}
	return errors.Join(errs...)
}

// peerTampers sends a new thread from the test peer whose attachment's
// declared hash is not the hash of its bytes.
func (w *world) peerTampers(ctx context.Context, list string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	peer, err := w.stack.sender(testPeer)
	if err != nil {
		return err
	}
	var recipients []string
	for _, p := range people {
		recipients = append(recipients, p.address)
	}
	e := peer.envelope("test@"+testPeer, "", "Contract", "The real one", recipients)
	e.attach("contract.pdf", "application/pdf", w.fileBytes("forged contract.pdf", 64<<10))
	e.Attachments[0].HashSHA256 = strings.Repeat("0", 64)
	r, err := peer.request(e)
	if err != nil {
		return err
	}
	w.lastAnswer, err = peer.send(ctx, people[0].domain, r)
	return err
}

// peerReplays sends a Domain a message as another Domain wrote it, signed
// with the test peer's own key. The envelope is rebuilt rather than copied:
// what is refused is the claim to speak for another Domain, and that does
// not depend on the bytes.
func (w *world) peerReplays(ctx context.Context, origin, destination string) error {
	peer, err := w.stack.sender(testPeer)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	threadID, recipients, err := w.threadAt(ctx, destination, c)
	if err != nil {
		return err
	}
	var author string
	for _, p := range c.members {
		if p.domain == origin {
			author = p.address
		}
	}
	e := peer.envelope(author, threadID, c.subject, w.lastReply, recipients)
	e.OriginDomain = origin
	r, err := peer.request(e)
	if err != nil {
		return err
	}
	w.lastAnswer, err = peer.send(ctx, destination, r)
	return err
}

// peerIntrudes sends a reply on a thread from someone at the test peer who
// was never on it, listing them as a member as if that made them one.
func (w *world) peerIntrudes(ctx context.Context, destination, subject, from string) error {
	peer, err := w.stack.sender(testPeer)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	threadID, recipients, err := w.threadAt(ctx, destination, c)
	if err != nil {
		return err
	}
	e := peer.envelope(from, threadID, subject, "I belong here", recipients)
	for _, p := range c.members {
		e.Thread.Members = append(e.Thread.Members, p.address)
	}
	r, err := peer.request(e)
	if err != nil {
		return err
	}
	w.lastAnswer, err = peer.send(ctx, destination, r)
	return err
}

// domainImpersonates has a real Domain, with its own key, send a reply on a
// thread as someone who lives on another Domain.
func (w *world) domainImpersonates(ctx context.Context, origin, destination, subject, as string) error {
	from, err := w.stack.sender(origin)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	threadID, recipients, err := w.threadAt(ctx, destination, c)
	if err != nil {
		return err
	}
	e := from.envelope(w.rewrite(as), threadID, subject, "It is me, really", recipients)
	r, err := from.request(e)
	if err != nil {
		return err
	}
	w.lastAnswer, err = from.send(ctx, destination, r)
	return err
}

func (w *world) answers(domain string, status int, code string) error {
	if w.lastAnswer.status != status || w.lastAnswer.code != code {
		return fmt.Errorf("%s answered %s, want %d %q", domain, w.lastAnswer, status, code)
	}
	return nil
}

// refusesToAdd checks the answer to a non-member writing into a thread: 425
// not_a_member, which keeps them out however often they retry (RFC 5.6).
func (w *world) refusesToAdd(domain, subject string) error {
	return w.answers(domain, 425, "not_a_member")
}

// ── Faults ──────────────────────────────────────────────────────────────────

func (w *world) arm(ctx context.Context, f fault) error {
	w.armed = true
	return w.stack.arm(ctx, f)
}

func (w *world) answerLost(ctx context.Context, destination, origin string) error {
	w.lostAt = destination
	return w.arm(ctx, fault{Domain: destination, From: origin, Action: "lose", Count: 1})
}

func (w *world) willRefuse(ctx context.Context, destination, origin string, status int, code string) error {
	return w.arm(ctx, fault{Domain: destination, From: origin, Action: "answer", Status: status, Error: code, Count: 1})
}

// retriesDelivery checks that a Domain's latest envelope for this story to
// the Domain whose answer was lost was delivered on a second or later
// attempt.
func (w *world) retriesDelivery(ctx context.Context, origin string) error {
	return eventually(ctx, 120, func() error {
		all, err := w.stack.envelopes(ctx, origin)
		if err != nil {
			return err
		}
		var last *envelope
		for i := range all {
			if w.ours(all[i].Recipients...) && all[i].PeerDomain == w.lostAt {
				last = &all[i]
			}
		}
		if last == nil {
			return fmt.Errorf("%s sent no envelope in this story", origin)
		}
		if last.Status != "delivered" || last.Attempts < 2 {
			return fmt.Errorf("envelope %d to %s is %s after %d attempts, want delivered on a retry",
				last.ID, last.PeerDomain, last.Status, last.Attempts)
		}
		return nil
	})
}

// ── Configuration ───────────────────────────────────────────────────────────

func (w *world) rotates(ctx context.Context, domain string) error {
	d, err := w.stack.descriptor(ctx, domain)
	if err != nil {
		return err
	}
	if _, ok := w.keyBefore[domain]; !ok {
		w.keyBefore[domain], _ = d["public_key"].(string)
	}
	cfg, err := w.stack.federationConfig(domain)
	if err != nil {
		return err
	}
	if !slices.Contains(w.reconfigured, domain) {
		w.reconfigured = append(w.reconfigured, domain)
	}
	path, err := w.stack.rotateKey(ctx, domain, cfg)
	if path != "" {
		w.newKeys = append(w.newKeys, path)
	}
	return err
}

// pinNewKey checks that each Domain now trusts the key a rotated Domain
// publishes, and that it is not the key it had before.
func (w *world) pinNewKey(ctx context.Context, list, rotated string) error {
	d, err := w.stack.descriptor(ctx, rotated)
	if err != nil {
		return err
	}
	current, _ := d["public_key"].(string)
	if current == "" || current == w.keyBefore[rotated] {
		return fmt.Errorf("%s still publishes the key it had before the story rotated it", rotated)
	}
	return eventually(ctx, 30, func() error {
		var errs []error
		for _, domain := range names(list) {
			pinned, err := w.stack.peers(ctx, domain)
			if err == nil && pinned[rotated].PublicKey != current {
				err = fmt.Errorf("%s pins %q for %s, want its new key %q", domain, pinned[rotated].PublicKey, rotated, current)
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

func (w *world) onlyAccepts(ctx context.Context, domain, list string) error {
	cfg, err := w.stack.federationConfig(domain)
	if err != nil {
		return err
	}
	cfg["allowedPeers"] = names(list)
	return w.reconfigure(ctx, domain, cfg)
}

// ── Outcomes ────────────────────────────────────────────────────────────────

// everyoneHasPinned waits until every member has the thread, and then checks
// that the author's Domain pinned each Domain it wrote to and each of those
// pinned the author's.
func (w *world) everyoneHasPinned(ctx context.Context, seconds int, subject string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	if err := w.untilEveryoneHas(ctx, c, ""); err != nil {
		return err
	}
	origin := c.author.domain
	var errs []error
	for _, domain := range w.domains() {
		if domain == origin {
			continue
		}
		for _, pair := range [][2]string{{origin, domain}, {domain, origin}} {
			pinned, err := w.stack.peers(ctx, pair[0])
			if err == nil && pinned[pair[1]].PublicKey == "" {
				err = fmt.Errorf("%s has not pinned %s", pair[0], pair[1])
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w *world) haveThread(ctx context.Context, seconds int, list, subject string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			threads, err := p.called(ctx, subject)
			if err == nil && len(threads) == 0 {
				err = fmt.Errorf("%s has no %q", p.name, subject)
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// stillExactly lets time pass, so anything a refused request might have set
// off has had its chance, and then checks that nothing arrived.
func (w *world) stillExactly(ctx context.Context, seconds, count int, subject string) error {
	if passTime != nil {
		if err := passTime(ctx, time.Duration(seconds)*time.Second); err != nil {
			return err
		}
	}
	// Real time for the workers to act on the clock's move.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range c.members {
		got, err := w.transcript(ctx, p, c)
		if err == nil && len(got) != count {
			err = fmt.Errorf("%s sees %d messages on %q, want exactly %d:\n  %s", p.name, len(got), subject, count,
				strings.Join(got, "\n  "))
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// membersUnchanged checks that every member still sees exactly the people
// the story put on the thread.
func (w *world) membersUnchanged(ctx context.Context, subject string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	var want []string
	for _, p := range c.members {
		want = append(want, p.address)
	}
	slices.Sort(want)
	var errs []error
	for _, p := range c.members {
		t, err := p.find(ctx, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		got := p.members(t)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			errs = append(errs, fmt.Errorf("%s sees members %v, want %v", p.name, got, want))
		}
	}
	return errors.Join(errs...)
}

// seeOnce checks that each person has the message with body exactly once on
// the last thread.
func (w *world) seeOnce(ctx context.Context, list, body string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		got, err := w.transcript(ctx, p, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		n := 0
		for _, line := range got {
			if strings.HasSuffix(line, ": "+body) {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, fmt.Errorf("%s sees %q %d times, want once", p.name, body, n))
		}
	}
	return errors.Join(errs...)
}

func (w *world) nobodyHearsFrom(ctx context.Context, address string) error {
	address = strings.ToLower(w.rewrite(address))
	var errs []error
	for _, p := range w.everyone() {
		view, err := p.view(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, t := range view {
			messages, err := p.session.messages(ctx, t.ThreadID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, m := range messages {
				if strings.EqualFold(m.SenderEmail, address) {
					errs = append(errs, fmt.Errorf("%s sees a message from %s in %q", p.name, address, t.Subject))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (w *world) notAMember(ctx context.Context, address, subject string) error {
	address = strings.ToLower(w.rewrite(address))
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range c.members {
		t, err := p.find(ctx, c)
		if err == nil && slices.Contains(p.members(t), address) {
			err = fmt.Errorf("%s sees %s as a member of %q", p.name, address, subject)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// stillSeeBody checks that each person still has a message on the last
// thread.
func (w *world) stillSeeBody(ctx context.Context, list, body string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		t, err := p.find(ctx, c)
		if err == nil {
			var shows bool
			if shows, err = w.shows(ctx, p, t, body); err == nil && !shows {
				err = fmt.Errorf("%s does not see %q", p.name, body)
			}
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (w *world) stillSeesCount(ctx context.Context, who string, count int, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	got, err := w.transcript(ctx, p, c)
	if err == nil && len(got) != count {
		err = fmt.Errorf("%s sees %d messages on %q, want still %d:\n  %s", p.name, len(got), subject, count,
			strings.Join(got, "\n  "))
	}
	return err
}

// bounceNaming checks for a bounce naming every listed person or quoted
// address and the reason.
func (w *world) bounceNaming(ctx context.Context, seconds int, who, subject, list, reason string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	people, others, err := w.recipients(list)
	if err != nil {
		return err
	}
	var addresses []string
	for _, n := range people {
		addresses = append(addresses, n.address)
	}
	for _, o := range others {
		addresses = append(addresses, w.rewrite(o))
	}
	return eventually(ctx, seconds, func() error {
		got, err := p.bounces(ctx, subject)
		if err != nil {
			return err
		}
		for _, b := range got {
			all := !slices.ContainsFunc(addresses, func(a string) bool { return !b.names(a) })
			if all && strings.Contains(b.body, "Reason: "+reason) {
				w.lastBounce = b
				return nil
			}
		}
		return fmt.Errorf("%s has no bounce %q naming %s for %s, has %+v", p.name, subject, list, reason, got)
	})
}

var discoveredAt = regexp.MustCompile(`(\S+) at (\S+)`)

// discovered checks where a Domain fetched each peer's descriptor: at the
// Federation hostname of the peer's own DNS domain, a subdomain no
// different from any other (RFC 4.1.1).
func (w *world) discovered(ctx context.Context, domain, list string) error {
	pinned, err := w.stack.peers(ctx, domain)
	if err != nil {
		return err
	}
	w.discoveredPeers = nil
	var errs []error
	for _, m := range discoveredAt.FindAllStringSubmatch(list, -1) {
		peer, host := m[1], m[2]
		w.discoveredPeers = append(w.discoveredPeers, pinned[peer])
		if want := "https://" + host + "/.well-known/webmail-federation"; pinned[peer].DescriptorURL != want {
			errs = append(errs, fmt.Errorf("%s discovered %s at %q, want %q", domain, peer, pinned[peer].DescriptorURL, want))
		}
		if pinned[peer].PublicKey == "" {
			errs = append(errs, fmt.Errorf("%s has not pinned %s", domain, peer))
		}
	}
	return errors.Join(errs...)
}

func (w *world) differentKeys() error {
	if len(w.discoveredPeers) != 2 {
		return fmt.Errorf("the story checked %d descriptors, not two", len(w.discoveredPeers))
	}
	a, b := w.discoveredPeers[0], w.discoveredPeers[1]
	if a.PublicKey == b.PublicKey {
		return fmt.Errorf("%s and %s carry the same key %s", a.Domain, b.Domain, a.PublicKey)
	}
	return nil
}

// noneUntilBack checks that a person whose Domain is down has nothing yet,
// because nothing can reach them, then brings the Domain back and waits
// until it has arrived.
func (w *world) noneUntilBack(ctx context.Context, who, subject, domain string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	if w.stack.health(ctx, domain) == nil {
		return fmt.Errorf("%s should be down, but answers", domain)
	}
	if err := w.comesBack(ctx, domain); err != nil {
		return err
	}
	// The attempt made while it was down is retried after a backoff of up to
	// a minute (RFC 6.4), so that is what "until it comes back" allows.
	return w.personHas(ctx, 120, p.name, subject)
}
