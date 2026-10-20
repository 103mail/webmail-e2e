package e2e

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// bounce is a delivery failure as the sender reads it: a thread called
// "Delivery failed: ..." whose message comes from the mailer daemon.
type bounce struct {
	subject string
	from    string
	body    string
}

// bounces lists a person's bounces, optionally only those with one subject.
func (p *person) bounces(ctx context.Context, subject string) ([]bounce, error) {
	view, err := p.view(ctx)
	if err != nil {
		return nil, err
	}
	var out []bounce
	for _, t := range view {
		if !strings.HasPrefix(t.Subject, "Delivery failed: ") || (subject != "" && t.Subject != subject) {
			continue
		}
		messages, err := p.session.messages(ctx, t.ThreadID)
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			continue
		}
		out = append(out, bounce{subject: t.Subject, from: strings.ToLower(messages[0].SenderEmail), body: messages[0].Body})
	}
	return out, nil
}

// failedRecipients reads the "Recipients:" line of a bounce.
var failedRecipients = regexp.MustCompile(`(?m)^Recipients: (.*)$`)

func (b bounce) names(address string) bool {
	m := failedRecipients.FindStringSubmatch(b.body)
	return m != nil && slices.Contains(strings.Split(m[1], ", "), address)
}

func (w *world) nobodyCalled(ctx context.Context, list, domain string) error {
	quoted := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list, -1)
	if len(quoted) == 0 {
		return fmt.Errorf("expected quoted addresses, got %s", list)
	}
	accounts, err := w.stack.accounts(ctx, domain)
	if err != nil {
		return err
	}
	var errs []error
	for _, q := range quoted {
		address := strings.ToLower(q[1])
		for _, a := range accounts {
			if a.Username == address || slices.Contains(a.Addresses, address) {
				errs = append(errs, fmt.Errorf("%s has an account for %s", domain, address))
			}
		}
	}
	return errors.Join(errs...)
}

func (w *world) haveInInbox(ctx context.Context, seconds int, list, subject string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			threads, err := p.called(ctx, subject)
			if err == nil && !slices.ContainsFunc(threads, func(t located) bool { return t.folder == "Inbox" }) {
				err = fmt.Errorf("%s has no %q in the Inbox", p.name, subject)
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// hasBounces checks that a person has exactly the bounces in the table, each
// from the right daemon, naming the failed recipient and the reason.
func (w *world) hasBounces(ctx context.Context, seconds int, who string, table *godog.Table) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	rows, err := tableRows(table)
	if err != nil {
		return err
	}
	if want := []string{"thread", "from", "failed recipient", "reason"}; !slices.Equal(rows[0], want) {
		return fmt.Errorf("unknown table %v; expected %v", rows[0], want)
	}
	return eventually(ctx, seconds, func() error {
		got, err := p.bounces(ctx, "")
		if err != nil {
			return err
		}
		var errs []error
		unmatched := slices.Clone(got)
		for _, row := range rows[1:] {
			from, recipient := w.rewrite(row[1]), w.rewrite(row[2])
			i := slices.IndexFunc(unmatched, func(b bounce) bool {
				return b.subject == row[0] && b.from == from && b.names(recipient) &&
					strings.Contains(b.body, "Reason: "+row[3])
			})
			if i < 0 {
				errs = append(errs, fmt.Errorf("%s has no bounce %q from %s for %s with reason %s",
					p.name, row[0], from, recipient, row[3]))
				continue
			}
			unmatched = slices.Delete(unmatched, i, i+1)
		}
		for _, b := range unmatched {
			errs = append(errs, fmt.Errorf("%s has an unexpected bounce %q:\n%s", p.name, b.subject, b.body))
		}
		if len(errs) == 0 {
			w.recordBounces(p, got)
		}
		return errors.Join(errs...)
	})
}

func (w *world) recordBounces(p *person, got []bounce) {
	counts := map[string]int{}
	for _, b := range got {
		counts[b.subject]++
	}
	w.bounceCounts[p.name] = counts
}

// oneBounceNamingUnknown checks for a single bounce naming every address the
// person's last message was written to that belongs to nobody in the story.
func (w *world) oneBounceNamingUnknown(ctx context.Context, seconds int, who, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	var unknown []string
	for _, c := range w.conversations {
		if c.author == p && "Delivery failed: "+c.subject == subject {
			unknown = c.others
		}
	}
	if len(unknown) == 0 {
		return fmt.Errorf("%s wrote no thread for %q with unknown addresses", who, subject)
	}
	return eventually(ctx, seconds, func() error {
		got, err := p.bounces(ctx, subject)
		if err != nil {
			return err
		}
		if len(got) != 1 {
			return fmt.Errorf("%s has %d bounces %q, want one", p.name, len(got), subject)
		}
		for _, address := range unknown {
			if !got[0].names(address) {
				return fmt.Errorf("the bounce does not name %s:\n%s", address, got[0].body)
			}
		}
		w.lastBounce = got[0]
		all, err := p.bounces(ctx, "")
		if err == nil {
			w.recordBounces(p, all)
		}
		return err
	})
}

func (w *world) bounceIsFrom(sender string) error {
	if w.lastBounce.subject == "" {
		return fmt.Errorf("no bounce was checked before this step")
	}
	if want := w.rewrite(sender); w.lastBounce.from != want {
		return fmt.Errorf("the bounce is from %s, want %s", w.lastBounce.from, want)
	}
	return nil
}

func (w *world) seeNoBounces(ctx context.Context, list string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		got, err := p.bounces(ctx, "")
		if err == nil && len(got) > 0 {
			err = fmt.Errorf("%s sees a bounce %q", p.name, got[0].subject)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// noFurtherBounce checks that the person has no more bounces for the subject
// than when their bounces were last checked.
func (w *world) noFurtherBounce(ctx context.Context, who, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	got, err := p.bounces(ctx, "Delivery failed: "+subject)
	if err != nil {
		return err
	}
	if before := w.bounceCounts[p.name]["Delivery failed: "+subject]; len(got) != before {
		return fmt.Errorf("%s has %d bounces for %q, had %d", p.name, len(got), subject, before)
	}
	return nil
}

func (w *world) seeTheReply(ctx context.Context, seconds int, list string) error {
	if w.lastReply == "" {
		return fmt.Errorf("nobody has replied yet")
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			t, err := p.find(ctx, c)
			if err == nil {
				var shows bool
				if shows, err = w.shows(ctx, p, t, w.lastReply); err == nil && !shows {
					err = fmt.Errorf("%s does not see %q", p.name, w.lastReply)
				}
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}
