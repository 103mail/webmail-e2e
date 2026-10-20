package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// registerMembership binds the phrases for joining, leaving and the things
// nobody can do to a thread (story 02).
func (w *world) registerMembership(sc *godog.ScenarioContext) {
	sc.Step(`^(\w+) wrote to (.+?): "([^"]*)"(?: with the body "([^"]*)")?$`, w.wrote)
	sc.Step(`^(\w+) replied to "([^"]*)": "([^"]*)"$`, w.replied)
	sc.Step(`^within (\d+) seconds (\w+) has "([^"]*)" in (?:his|her|their) Inbox with (\d+) messages, the last saying (\w+) added (?:him|her|them)$`, w.hasInvitation)
	sc.Step(`^within (\d+) seconds (.+?)(?: each)? (?:has|have) "([^"]*)" with (\d+) messages$`, w.haveMessages)
	sc.Step(`^(\w+) can read "([^"]*)"$`, w.canRead)
	sc.Step(`^within (\d+) seconds every (?:remaining )?member (?:still )?sees (.+?) as members(?: of "([^"]*)")?$`, w.eventuallyMembers)
	sc.Step(`^(\w+) leaves "([^"]*)"$`, w.leaves)
	sc.Step(`^within (\d+) seconds ([\w ,]+?) sees? "([^"]*)"$`, w.eventuallySee)
	sc.Step(`^(\w+) does not see "([^"]*)"$`, func(ctx context.Context, who, body string) error { return w.neverSee(ctx, who, body) })
	sc.Step(`^(\w+) tries to reply to "([^"]*)": "([^"]*)"$`, w.triesToReply)
	sc.Step(`^(\w+)'s reply is refused because (?:he|she|they) (?:is|are) no longer a member$`,
		func() error { return w.refusedWith(http.StatusForbidden, "not_a_member") })
	sc.Step(`^nobody sees "([^"]*)"$`, w.nobodySees)
	sc.Step(`^(\w+) tries to remove (\w+) from "([^"]*)"$`, w.triesToRemove)
	sc.Step(`^(\w+) is told that only (\w+) can remove (?:himself|herself|themselves)$`,
		func() error { return w.refusedWith(http.StatusForbidden, "only_yourself") })
	sc.Step(`^(\w+) asks for a way to forward "([^"]*)" to (\w+)$`, w.asksToForward)
	sc.Step(`^the API offers no forward action, only an invitation$`, w.noForward)
	sc.Step(`^(\w+) quotes "([^"]*)" in a new message to (.+): "([^"]*)"$`, w.quotes)
	sc.Step(`^within (\d+) seconds (.+) have the thread "([^"]*)" containing the quoted text$`, w.haveQuote)
	sc.Step(`^(.+) (?:has|have) no access to "([^"]*)"$`, w.haveNoThread)
	sc.Step(`^"([^"]*)" still lists only (.+) as members$`,
		func(ctx context.Context, subject, list string) error { return w.membersAre(ctx, list, subject) })
}

// wrote is "writes to" as a Given: it waits until everyone written to has it.
func (w *world) wrote(ctx context.Context, author, list, subject, body string) error {
	if err := w.writes(ctx, author, list, subject, body); err != nil {
		return err
	}
	c, err := w.conversation(author, subject)
	if err != nil {
		return err
	}
	return w.untilEveryoneHas(ctx, c, "")
}

// replied is "replies to" as a Given: it waits until the replier has the
// thread, and then until every member has the reply.
func (w *world) replied(ctx context.Context, who, subject, body string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	p, err := w.person(who)
	if err != nil {
		return err
	}
	if err := eventually(ctx, 30, func() error { _, err := p.find(ctx, c); return err }); err != nil {
		return err
	}
	if err := w.replies(ctx, who, "", subject, body); err != nil {
		return err
	}
	return w.untilEveryoneHas(ctx, c, body)
}

// untilEveryoneHas waits until every member has the thread, and the message
// with body when one is given.
func (w *world) untilEveryoneHas(ctx context.Context, c *conversation, body string) error {
	return eventually(ctx, 30, func() error {
		var errs []error
		for _, p := range c.members {
			t, err := p.find(ctx, c)
			if err == nil && body != "" {
				var shows bool
				if shows, err = w.shows(ctx, p, t, body); err == nil && !shows {
					err = fmt.Errorf("%s does not have %q yet", p.name, body)
				}
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

func (w *world) hasInvitation(ctx context.Context, seconds int, who, subject string, count int, inviter string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	by, err := w.person(inviter)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		t, err := p.find(ctx, c)
		if err != nil {
			return err
		}
		if t.folder != "Inbox" {
			return fmt.Errorf("%s has %q in %s", p.name, subject, t.folder)
		}
		messages, err := p.session.messages(ctx, t.ThreadID)
		if err != nil {
			return err
		}
		if len(messages) != count {
			return fmt.Errorf("%s has %d messages in %q, want %d", p.name, len(messages), subject, count)
		}
		last := messages[len(messages)-1]
		if strings.ToLower(last.SenderEmail) != by.address || !strings.Contains(last.Body, p.address) {
			return fmt.Errorf("the last message is from %s: %q, want %s adding %s",
				last.SenderEmail, last.Body, by.address, p.address)
		}
		return nil
	})
}

func (w *world) haveMessages(ctx context.Context, seconds int, list, subject string, count int) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			t, err := p.find(ctx, c)
			if err == nil {
				var messages []message
				if messages, err = p.session.messages(ctx, t.ThreadID); err == nil && len(messages) != count {
					err = fmt.Errorf("%s has %d messages in %q, want %d", p.name, len(messages), subject, count)
				}
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

func (w *world) canRead(ctx context.Context, who, body string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	t, err := p.find(ctx, c)
	if err != nil {
		return err
	}
	shows, err := w.shows(ctx, p, t, body)
	if err == nil && !shows {
		err = fmt.Errorf("%s cannot read %q", p.name, body)
	}
	return err
}

func (w *world) eventuallyMembers(ctx context.Context, seconds int, list, subject string) error {
	if subject == "" {
		subject = w.lastSubject
	}
	return eventually(ctx, seconds, func() error { return w.membersAre(ctx, list, subject) })
}

// leaves takes the person off the thread through their own Domain, and out
// of the members the story expects from then on.
func (w *world) leaves(ctx context.Context, who, subject string) error {
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
	if err := p.session.leave(ctx, t.ThreadID, p.address); err != nil {
		return fmt.Errorf("%s leaves %q: %w", who, subject, err)
	}
	c.left = append(c.left, p)
	c.members = slices.DeleteFunc(c.members, func(m *person) bool { return m == p })
	w.lastSubject = subject
	return nil
}

func (w *world) eventuallySee(ctx context.Context, seconds int, list, body string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
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
	})
}

// threadOf finds a person's copy of a thread they may have left, so the
// story can try things on it.
func (w *world) threadOf(ctx context.Context, p *person, subject string) (located, error) {
	threads, err := p.called(ctx, subject)
	if err != nil {
		return located{}, err
	}
	if len(threads) != 1 {
		return located{}, fmt.Errorf("%s has %d threads %q", p.name, len(threads), subject)
	}
	return threads[0], nil
}

func (w *world) triesToReply(ctx context.Context, who, subject, body string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	t, err := w.threadOf(ctx, p, subject)
	if err != nil {
		return err
	}
	w.lastAttempt.status, w.lastAttempt.code, err = p.session.try(ctx, http.MethodPost,
		fmt.Sprintf("/api/v1/email/message/%d", t.ThreadID), map[string]any{"body": body})
	return err
}

func (w *world) triesToRemove(ctx context.Context, who, target, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	victim, err := w.person(target)
	if err != nil {
		return err
	}
	t, err := w.threadOf(ctx, p, subject)
	if err != nil {
		return err
	}
	w.lastAttempt.status, w.lastAttempt.code, err = p.session.try(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v1/email/subscriber/%d?email=%s", t.ThreadID, victim.address), nil)
	return err
}

func (w *world) refusedWith(status int, code string) error {
	if w.lastAttempt.status != status || w.lastAttempt.code != code {
		return fmt.Errorf("the attempt was answered %d %q, want %d %q",
			w.lastAttempt.status, w.lastAttempt.code, status, code)
	}
	return nil
}

func (w *world) nobodySees(ctx context.Context, body string) error {
	var names []string
	for _, p := range w.everyone() {
		names = append(names, p.name)
	}
	return w.neverSee(ctx, strings.Join(names, ", "), body)
}

// asksToForward looks for a forward action. There must be none: the only
// way to show a thread to someone is to invite them (principle 1).
func (w *world) asksToForward(ctx context.Context, who, subject, target string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	to, err := w.person(target)
	if err != nil {
		return err
	}
	t, err := w.threadOf(ctx, p, subject)
	if err != nil {
		return err
	}
	w.lastAttempt.status, w.lastAttempt.code, err = p.session.try(ctx, http.MethodPost,
		fmt.Sprintf("/api/v1/email/forward/%d", t.ThreadID), map[string]any{"email": to.address})
	return err
}

func (w *world) noForward() error {
	if w.lastAttempt.status != http.StatusNotFound {
		return fmt.Errorf("asking to forward was answered %d, want 404: there is no forward action", w.lastAttempt.status)
	}
	return nil
}

// quotes writes a new thread whose body quotes text from another one. It
// carries the text only: no link or reference to where it came from.
func (w *world) quotes(ctx context.Context, author, quote, list, subject string) error {
	w.lastQuote = quote
	return w.writes(ctx, author, list, subject, "<blockquote>"+quote+"</blockquote>")
}

func (w *world) haveQuote(ctx context.Context, seconds int, list, subject string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			threads, err := p.called(ctx, subject)
			if err == nil && len(threads) != 1 {
				err = fmt.Errorf("%s has %d threads %q", p.name, len(threads), subject)
			}
			if err == nil {
				var messages []message
				if messages, err = p.session.messages(ctx, threads[0].ThreadID); err == nil &&
					!slices.ContainsFunc(messages, func(m message) bool { return strings.Contains(m.Body, w.lastQuote) }) {
					err = fmt.Errorf("%s's %q does not contain %q", p.name, subject, w.lastQuote)
				}
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}
