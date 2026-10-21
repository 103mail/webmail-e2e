package e2e

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// register binds the step vocabulary from the README to this world.
func (w *world) register(sc *godog.ScenarioContext) {
	// People. The account phrase takes only names, so that "nobody called
	// "x@y" has an account" is not also read as a list of names.
	sc.Step(`^nobody called (.+) has an account on (\S+)$`, w.nobodyCalled)
	sc.Step(`^(\w+(?:, \w+)*(?: and \w+)?) (?:has an account|have accounts) on (\S+)$`, w.haveAccounts)

	// Acts
	sc.Step(`^(\w+) writes to (.+?): "([^"]*)"(?: with the body "([^"]*)")?$`, w.writes)
	sc.Step(`^(\w+) replies to (?:(\w+)'s )?"([^"]*)": "([^"]*)"$`, w.replies)
	sc.Step(`^(\w+) invites (.+) to (?:(\w+)'s )?"([^"]*)"$`, w.invites)
	sc.Step(`^(\w+) reads "([^"]*)"$`, w.reads)

	// Outcomes
	sc.Step(`^(\w+)'s send is confirmed within (\d+) seconds?$`, w.sendConfirmed)
	sc.Step(`^within (\d+) seconds everyone sees:$`, w.everyoneSees)
	sc.Step(`^(.+) (?:has|have) no thread "([^"]*)"$`, w.haveNoThread)
	sc.Step(`^within (\d+) seconds the thread "([^"]*)" reads the same for everyone:$`, w.readsTheSame)
	sc.Step(`^every member sees (.+) as members of "([^"]*)"$`, w.membersAre)
	sc.Step(`^"([^"]*)" is read for (.+) and still unread for (.+)$`, w.readAndUnread)
	sc.Step(`^read state never crossed a Domain: (.+) still sees? it as unread$`, w.stillUnread)
	sc.Step(`^within (\d+) seconds "([^"]*)" is unread again for (.+)$`, w.unreadAgain)
	sc.Step(`^each member has exactly one thread called "([^"]*)"$`, w.exactlyOne)
	sc.Step(`^within (\d+) seconds only (\w+)'s thread with (.+) shows "([^"]*)"$`, w.onlyThreadShows)
	sc.Step(`^(.+) never sees? "([^"]*)"$`, w.neverSee)
	sc.Step(`^every person's contact suggestions now include everyone they exchanged mail with$`, w.contactsInclude)
	sc.Step(`^no email has left any Domain containing:$`, w.noEmailContaining)
	sc.Step(`^(\S+) delivered exactly (.+)$`, w.deliveredExactly)
	sc.Step(`^(\S+) holds accounts only for its own people$`, w.ownPeopleOnly)

	// Delivery and bounces
	sc.Step(`^within (\d+) seconds (.+?)(?: each)? (?:has|have) "([^"]*)" in (?:his|her|their) Inbox$`, w.haveInInbox)
	sc.Step(`^within (\d+) seconds (\w+) has these bounces in (?:his|her|their) Inbox:$`, w.hasBounces)
	sc.Step(`^within (\d+) seconds (\w+) has one bounce "([^"]*)" naming both unknown addresses$`, w.oneBounceNamingUnknown)
	sc.Step(`^it is from "([^"]*)"$`, w.bounceIsFrom)
	sc.Step(`^the bounces exist only on \S+: (.+) never sees? them$`, w.seeNoBounces)
	sc.Step(`^(\w+) gets no further bounce for "([^"]*)"$`, w.noFurtherBounce)
	sc.Step(`^within (\d+) seconds every member of "([^"]*)" sees (.+) as the people who received it$`,
		func(ctx context.Context, seconds int, subject, list string) error {
			return eventually(ctx, seconds, func() error { return w.membersAre(ctx, list, subject) })
		})
	sc.Step(`^within (\d+) seconds (.+) sees? the reply$`, w.seeTheReply)

	w.registerMembership(sc)
	w.registerAttachments(sc)
	w.registerTrust(sc)
	w.registerOutage(sc)
	w.registerCatchUp(sc)
	w.registerRateLimit(sc)

	// Email checks look only at what was sent during this story.
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.firstEmail, w.firstEmailErr = w.stack.lastEmail(ctx)
		return ctx, nil
	})
}

// pending marks a step whose phrase is agreed but whose harness is not built.
func pending(reason string) func() error {
	return func() error { return fmt.Errorf("%w: %s", godog.ErrPending, reason) }
}

// ── People ──────────────────────────────────────────────────────────────────

func (w *world) haveAccounts(ctx context.Context, list, domain string) error {
	if err := w.stack.Check(ctx, domain); err != nil {
		return err
	}
	for _, name := range names(list) {
		if _, ok := w.people[name]; ok {
			return fmt.Errorf("%s already has an account in this story", name)
		}
		address := w.addressFor(name, domain)
		if err := w.stack.Provision(ctx, domain, address, name); err != nil {
			return err
		}
		session, err := w.stack.Login(ctx, domain, address)
		if err != nil {
			return err
		}
		w.people[name] = &person{name: name, domain: domain, address: address, session: session}
	}
	return nil
}

// ── Acts ────────────────────────────────────────────────────────────────────

func (w *world) writes(ctx context.Context, author, list, subject, body string) error {
	return w.writeWith(ctx, author, list, subject, body)
}

// writeWith starts a thread, with files attached when there are any.
func (w *world) writeWith(ctx context.Context, author, list, subject, body string, files ...upload) error {
	p, err := w.person(author)
	if err != nil {
		return err
	}
	recipients, others, err := w.recipients(list)
	if err != nil {
		return err
	}
	folderID, err := p.defaultFolder(ctx)
	if err != nil {
		return err
	}
	addresses := make([]string, len(recipients))
	for i, r := range recipients {
		addresses[i] = r.address
	}
	addresses = append(addresses, others...)
	start := time.Now()
	if _, err := p.session.addThread(ctx, folderID, addresses, subject, body, files...); err != nil {
		return fmt.Errorf("%s writes %q: %w", author, subject, err)
	}
	w.lastSend.author, w.lastSend.took = author, time.Since(start)
	w.sends = append(w.sends, w.lastSend.took)
	w.conversations = append(w.conversations, &conversation{
		author: p, subject: subject, body: body, members: append([]*person{p}, recipients...),
		writers: []*person{p}, others: others,
	})
	w.lastSubject = subject
	return nil
}

func (w *world) replies(ctx context.Context, who, owner, subject, body string) error {
	return w.replyWith(ctx, who, owner, subject, body)
}

// replyWith adds a message to a thread, with files attached when there are
// any.
func (w *world) replyWith(ctx context.Context, who, owner, subject, body string, files ...upload) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation(owner, subject)
	if err != nil {
		return err
	}
	t, err := p.find(ctx, c)
	if err != nil {
		return err
	}
	start := time.Now()
	if _, err := p.session.addMessage(ctx, t.ThreadID, body, files...); err != nil {
		return fmt.Errorf("%s replies to %q: %w", who, subject, err)
	}
	w.sends = append(w.sends, time.Since(start))
	c.wrote(p)
	w.lastSubject = subject
	w.lastReply = body
	w.replyBy[p.name] = body
	w.replyBodies = append(w.replyBodies, body)
	return nil
}

func (w *world) invites(ctx context.Context, who, list, owner, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	invitees, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation(owner, subject)
	if err != nil {
		return err
	}
	t, err := p.find(ctx, c)
	if err != nil {
		return err
	}
	for _, i := range invitees {
		if _, err := p.session.addSubscriber(ctx, t.ThreadID, i.address); err != nil {
			return fmt.Errorf("%s invites %s to %q: %w", who, i.name, subject, err)
		}
		if !c.has(i) {
			c.members = append(c.members, i)
		}
	}
	w.lastSubject = subject
	return nil
}

// reads opens the thread the way the client does: list it, then mark each
// unread message read.
func (w *world) reads(ctx context.Context, who, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	t, err := p.find(ctx, c)
	if err != nil {
		return err
	}
	messages, err := p.session.messages(ctx, t.ThreadID)
	if err != nil {
		return err
	}
	for _, m := range messages {
		if m.ReadAt == nil {
			if err := p.session.markRead(ctx, m.MessageID); err != nil {
				return fmt.Errorf("%s reads %q: %w", who, subject, err)
			}
		}
	}
	w.lastSubject = subject
	return nil
}

// ── Outcomes ────────────────────────────────────────────────────────────────

func (w *world) sendConfirmed(author string, seconds int) error {
	if w.lastSend.author != author {
		return fmt.Errorf("the last send was by %q, not %s", w.lastSend.author, author)
	}
	if limit := time.Duration(seconds) * time.Second; w.lastSend.took > limit {
		return fmt.Errorf("%s's send took %s, more than %s", author, w.lastSend.took.Round(time.Millisecond), limit)
	}
	return nil
}

var threadsCalled = regexp.MustCompile(`^threads called "([^"]*)"$`)

// everyoneSees takes one of two tables: per person the folder, thread,
// message count and unread state, or per person how many threads carry a
// subject.
func (w *world) everyoneSees(ctx context.Context, seconds int, table *godog.Table) error {
	rows, err := tableRows(table)
	if err != nil {
		return err
	}
	header := rows[0]
	if len(header) == 2 && header[0] == "person" && threadsCalled.MatchString(header[1]) {
		subject := threadsCalled.FindStringSubmatch(header[1])[1]
		return eventually(ctx, seconds, func() error {
			return w.checkRows(rows[1:], func(p *person, row []string) error {
				return w.checkThreadCount(ctx, p, subject, row[1])
			})
		})
	}
	want := []string{"person", "folder", "thread", "messages", "unread"}
	if !slices.Equal(header, want) {
		return fmt.Errorf("unknown table %v; expected %v or [person, threads called \"...\"]", header, want)
	}
	return eventually(ctx, seconds, func() error {
		return w.checkRows(rows[1:], func(p *person, row []string) error {
			return w.checkThreadRow(ctx, p, row[1], row[2], row[3], row[4])
		})
	})
}

// checkRows runs check for every row and joins what failed, so one poll
// reports every person who does not see the right thing yet.
func (w *world) checkRows(rows [][]string, check func(*person, []string) error) error {
	var errs []error
	for _, row := range rows {
		p, err := w.person(row[0])
		if err == nil {
			err = check(p, row)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (w *world) checkThreadRow(ctx context.Context, p *person, folder, subject, messages, unread string) error {
	threads, err := p.called(ctx, subject)
	if err != nil {
		return err
	}
	if len(threads) != 1 {
		return fmt.Errorf("%s has %d threads %q, expected 1", p.name, len(threads), subject)
	}
	t := threads[0]
	if t.folder != folder {
		return fmt.Errorf("%s has %q in %s, expected %s", p.name, subject, t.folder, folder)
	}
	list, err := p.session.messages(ctx, t.ThreadID)
	if err != nil {
		return err
	}
	if strconv.Itoa(len(list)) != messages {
		return fmt.Errorf("%s has %d messages in %q, expected %s", p.name, len(list), subject, messages)
	}
	if got := yesNo(t.UnreadCount > 0); got != unread {
		return fmt.Errorf("%s: %q unread is %s, expected %s", p.name, subject, got, unread)
	}
	return nil
}

func (w *world) checkThreadCount(ctx context.Context, p *person, subject, want string) error {
	threads, err := p.called(ctx, subject)
	if err != nil {
		return err
	}
	if strconv.Itoa(len(threads)) != want {
		return fmt.Errorf("%s has %d threads called %q, expected %s", p.name, len(threads), subject, want)
	}
	return nil
}

func (w *world) haveNoThread(ctx context.Context, list, subject string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		errs = append(errs, w.checkThreadCount(ctx, p, subject, "0"))
	}
	return errors.Join(errs...)
}

// readsTheSame checks that every member of the thread sees exactly the
// messages in the table, in that order, from those senders.
func (w *world) readsTheSame(ctx context.Context, seconds int, subject string, table *godog.Table) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	rows, err := tableRows(table)
	if err != nil {
		return err
	}
	if !slices.Equal(rows[0], []string{"from", "body"}) {
		return fmt.Errorf("unknown table %v; expected [from body]", rows[0])
	}
	var want []string
	for _, row := range rows[1:] {
		body := row[1]
		if body == "(kick-off)" {
			// The message that started the thread, whatever it said.
			body = c.body
		}
		want = append(want, w.rewrite(row[0])+": "+body)
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range c.members {
			got, err := w.transcript(ctx, p, c)
			if err == nil && !slices.Equal(got, want) {
				err = fmt.Errorf("%s reads:\n  %s\nexpected:\n  %s", p.name,
					strings.Join(got, "\n  "), strings.Join(want, "\n  "))
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// transcript is a thread as one person reads it, oldest first, one
// "sender: body" line per message.
func (w *world) transcript(ctx context.Context, p *person, c *conversation) ([]string, error) {
	t, err := p.find(ctx, c)
	if err != nil {
		return nil, err
	}
	messages, err := p.session.messages(ctx, t.ThreadID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(messages))
	for i, m := range messages {
		out[i] = strings.ToLower(m.SenderEmail) + ": " + strings.TrimSpace(m.Body)
	}
	return out, nil
}

func (w *world) membersAre(ctx context.Context, list, subject string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	var want []string
	for _, p := range people {
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
			errs = append(errs, fmt.Errorf("%s sees members %v, expected %v", p.name, got, want))
		}
	}
	return errors.Join(errs...)
}

// checkUnread compares each person's unread state of a thread with want.
func (w *world) checkUnread(ctx context.Context, c *conversation, people []*person, want bool) error {
	var errs []error
	for _, p := range people {
		t, err := p.find(ctx, c)
		if err == nil && (t.UnreadCount > 0) != want {
			err = fmt.Errorf("%s: %q unread is %s, expected %s", p.name, c.subject,
				yesNo(t.UnreadCount > 0), yesNo(want))
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (w *world) readAndUnread(ctx context.Context, subject, readList, unreadList string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	read, err := w.persons(readList)
	if err != nil {
		return err
	}
	unread, err := w.persons(unreadList)
	if err != nil {
		return err
	}
	return errors.Join(w.checkUnread(ctx, c, read, false), w.checkUnread(ctx, c, unread, true))
}

func (w *world) stillUnread(ctx context.Context, list string) error {
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return w.checkUnread(ctx, c, people, true)
}

func (w *world) unreadAgain(ctx context.Context, seconds int, subject, list string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		return w.checkUnread(ctx, c, people, true)
	})
}

func (w *world) exactlyOne(ctx context.Context, subject string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range c.members {
		errs = append(errs, w.checkThreadCount(ctx, p, subject, "1"))
	}
	return errors.Join(errs...)
}

// onlyThreadShows checks that a message reached every member of one thread
// and appears in no other thread anyone in the story has.
func (w *world) onlyThreadShows(ctx context.Context, seconds int, owner, list, body string) error {
	author, err := w.person(owner)
	if err != nil {
		return err
	}
	with, err := w.persons(list)
	if err != nil {
		return err
	}
	var target *conversation
	for _, c := range w.conversations {
		if c.author == author && len(c.members) == len(with)+1 &&
			!slices.ContainsFunc(with, func(p *person) bool { return !c.has(p) }) {
			target = c
		}
	}
	if target == nil {
		return fmt.Errorf("%s started no thread with exactly %s", owner, list)
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range w.everyone() {
			view, err := p.view(ctx)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			own := located{}
			if target.has(p) {
				if own, err = p.find(ctx, target); err != nil {
					errs = append(errs, err)
					continue
				}
			}
			for _, t := range view {
				shows, err := w.shows(ctx, p, t, body)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				isTarget := target.has(p) && t.ThreadID == own.ThreadID
				if shows != isTarget {
					errs = append(errs, fmt.Errorf("%s: %q shows %q is %s, expected %s",
						p.name, t.Subject, body, yesNo(shows), yesNo(isTarget)))
				}
			}
		}
		return errors.Join(errs...)
	})
}

// shows reports whether a thread in a person's view has a message with body.
func (w *world) shows(ctx context.Context, p *person, t located, body string) (bool, error) {
	messages, err := p.session.messages(ctx, t.ThreadID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(messages, func(m message) bool {
		return strings.TrimSpace(m.Body) == body
	}), nil
}

func (w *world) neverSee(ctx context.Context, list, body string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		view, err := p.view(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, t := range view {
			shows, err := w.shows(ctx, p, t, body)
			if err == nil && shows {
				err = fmt.Errorf("%s sees %q in %q", p.name, body, t.Subject)
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// contactsInclude checks that everyone has a contact for each person they
// exchanged mail with: everyone on a thread they wrote in, and everyone who
// wrote in a thread they are on. Two people who were only ever recipients on
// the same thread have not exchanged mail.
func (w *world) contactsInclude(ctx context.Context) error {
	var errs []error
	for _, p := range w.everyone() {
		contacts, err := p.session.contacts(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		have := map[string]bool{}
		for _, c := range contacts {
			have[strings.ToLower(c.Email)] = true
		}
		for _, c := range w.conversations {
			if !c.has(p) {
				continue
			}
			others := c.writers
			if slices.Contains(c.writers, p) {
				others = c.members
			}
			for _, m := range others {
				if m != p && !have[m.address] {
					errs = append(errs, fmt.Errorf("%s has no contact for %s", p.name, m.name))
					have[m.address] = true // report each missing contact once
				}
			}
		}
	}
	return errors.Join(errs...)
}

// noEmailContaining checks every email any Domain sent during the story,
// headers and body, for each of the phrases in the table.
func (w *world) noEmailContaining(ctx context.Context, table *godog.Table) error {
	if w.firstEmailErr != nil {
		return w.firstEmailErr
	}
	if table == nil || len(table.Rows) == 0 {
		return fmt.Errorf("the step needs a table of phrases")
	}
	emails, err := w.stack.emailsAfter(ctx, w.firstEmail)
	if err != nil {
		return err
	}
	var errs []error
	for _, row := range table.Rows {
		phrase := strings.TrimSpace(row.Cells[0].Value)
		for _, e := range emails {
			n := e.Notification
			for _, part := range []string{n.Subject, n.Text, n.HTML, string(n.Raw)} {
				if strings.Contains(part, phrase) {
					errs = append(errs, fmt.Errorf("email %d to %v contains %q", e.ID, n.To, phrase))
					break
				}
			}
		}
	}
	return errors.Join(errs...)
}

// envelopeCount reads one "one envelope to example.com" clause.
var envelopeCount = regexp.MustCompile(`^(one|two|three|four|five|\d+)(?: envelopes?)? to (\S+)$`)

var numbers = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5}

// deliveredExactly checks a Domain's outbox for this story's envelopes, as in
// "103mail.com delivered exactly one envelope to example.com and one to
// mail.acme.test". Every envelope must be delivered, and no peer that is not
// named may have been sent one.
func (w *world) deliveredExactly(ctx context.Context, origin, clauses string) error {
	want := map[string]int{}
	for _, clause := range strings.Split(clauses, " and ") {
		m := envelopeCount.FindStringSubmatch(strings.TrimSpace(clause))
		if m == nil {
			return fmt.Errorf("cannot read %q; expected \"one envelope to example.com\"", clause)
		}
		n, ok := numbers[m[1]]
		if !ok {
			n, _ = strconv.Atoi(m[1])
		}
		want[m[2]] = n
	}
	all, err := w.stack.envelopes(ctx, origin)
	if err != nil {
		return err
	}
	got := map[string]int{}
	var errs []error
	for _, e := range all {
		if !w.ours(e.Recipients...) {
			continue
		}
		got[e.PeerDomain]++
		if e.Status != "delivered" {
			errs = append(errs, fmt.Errorf("envelope %d to %s is %s", e.ID, e.PeerDomain, e.Status))
		}
	}
	for peer, n := range got {
		if _, named := want[peer]; !named {
			errs = append(errs, fmt.Errorf("%s sent %d envelopes to %s, expected none", origin, n, peer))
		}
	}
	for peer, n := range want {
		if got[peer] != n {
			errs = append(errs, fmt.Errorf("%s sent %d envelopes to %s, expected %d", origin, got[peer], peer, n))
		}
	}
	return errors.Join(errs...)
}

// ownPeopleOnly checks that every account on a Domain that belongs to this
// story is for one of its own people: someone on another Domain must never
// get an account, or an address, here.
func (w *world) ownPeopleOnly(ctx context.Context, domain string) error {
	accounts, err := w.stack.accounts(ctx, domain)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range accounts {
		addresses := append([]string{a.Username}, a.Addresses...)
		if !w.ours(addresses...) {
			continue
		}
		for _, address := range addresses {
			if !strings.HasSuffix(strings.ToLower(address), "@"+domain) {
				errs = append(errs, fmt.Errorf("%s holds the account %s with the address %s",
					domain, a.Username, address))
			}
		}
	}
	return errors.Join(errs...)
}

// ours reports whether any of the addresses was made for this story run.
func (w *world) ours(addresses ...string) bool {
	return slices.ContainsFunc(addresses, func(a string) bool {
		return strings.Contains(strings.ToLower(a), "."+w.run+"@")
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func tableRows(table *godog.Table) ([][]string, error) {
	if table == nil || len(table.Rows) < 2 {
		return nil, fmt.Errorf("the step needs a table with a header and at least one row")
	}
	out := make([][]string, len(table.Rows))
	for i, r := range table.Rows {
		for _, c := range r.Cells {
			out[i] = append(out[i], strings.TrimSpace(c.Value))
		}
	}
	return out, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
