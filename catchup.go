package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCatchUp binds the phrases for a thread catching up after an
// outage: counts per person, order, and what was not held up (story 04).
func (w *world) registerCatchUp(sc *godog.ScenarioContext) {
	sc.Step(`^within (\d+) seconds everyone has "([^"]*)" with (\d+) messages?$`,
		func(ctx context.Context, seconds int, subject string, count int) error {
			return w.membersSee(ctx, seconds, "everyone", count, subject, false)
		})
	sc.Step(`^every send is confirmed within (\d+) seconds?$`, w.sendsConfirmed)
	sc.Step(`^within (\d+) (seconds?|minutes?) (.+?) sees? (\d+) messages? on "([^"]*)"(, in the same order)?$`,
		func(ctx context.Context, n int, unit, list string, count int, subject, ordered string) error {
			return w.membersSee(ctx, toSeconds(n, unit), list, count, subject, ordered != "")
		})
	sc.Step(`^within (\d+) (seconds?|minutes?) ([\w ,]+?) sees? "([^"]*)" with (\d+) messages?$`,
		func(ctx context.Context, n int, unit, list, subject string, count int) error {
			return w.membersSee(ctx, toSeconds(n, unit), list, count, subject, false)
		})
	sc.Step(`^mail between (\S+) and (\S+) was not delayed by \S+$`, w.notDelayed)
	sc.Step(`^the thread "([^"]*)" reads the same for everyone:$`,
		func(ctx context.Context, subject string, table *godog.Table) error {
			return w.readsTheSame(ctx, 0, subject, table)
		})
	sc.Step(`^([\w ,]+?) still sees? (\d+) messages?$`, w.stillSee)
	sc.Step(`^(\w+)'s reply appears after (\w+)'s and (\w+)'s for everyone$`, w.replyAfter)

	// A peer that asks the others to slow down (RFC 6.4).
	sc.Step(`^(\S+) answers every delivery with "([^"]*)" and a retry after (\d+) seconds$`,
		func(ctx context.Context, domain, code string, seconds int) error {
			return w.arm(ctx, fault{Domain: domain, Action: "answer", Status: http.StatusTooManyRequests,
				Error: code, RetryAfter: seconds})
		})
	sc.Step(`^(\S+) stops rate limiting$`, func(ctx context.Context, domain string) error {
		return w.stack.clearFaults(ctx, domain)
	})
	sc.Step(`^(.+) make no delivery attempt to (\S+) for (\d+) seconds$`, w.holdOff)
	sc.Step(`^within (\d+) (seconds?|minutes?) (.+) sees? both replies$`,
		func(ctx context.Context, n int, unit, list string) error {
			if len(w.replyBodies) < 2 {
				return fmt.Errorf("the story has %d replies, not two", len(w.replyBodies))
			}
			seconds := toSeconds(n, unit)
			for _, body := range w.replyBodies[len(w.replyBodies)-2:] {
				if err := w.eventuallySee(ctx, seconds, list, body); err != nil {
					return err
				}
			}
			return nil
		})
}

// holdOff checks that every listed Domain was turned away by the peer once
// and then left it alone for the time it asked for. Time passes on the
// stack's clock, which also runs on in real time while the step polls, so
// both count; it stops ten seconds short, so the check is about the wait the
// peer asked for and not the retry just after it.
func (w *world) holdOff(ctx context.Context, origins, peer string, seconds int) error {
	attempts := func(origin string) (map[int64]int, error) {
		all, err := w.stack.envelopes(ctx, origin)
		if err != nil {
			return nil, err
		}
		out := map[int64]int{}
		for _, e := range all {
			if e.PeerDomain == peer && w.ours(e.Recipients...) {
				out[e.ID] = e.Attempts
			}
		}
		return out, nil
	}
	domains := names(origins)
	before := map[string]map[int64]int{}
	// The attempts happened no earlier than the replies just before this step.
	start := time.Now()
	var moved time.Duration
	elapsed := func() time.Duration { return time.Since(start) + moved }
	limit := time.Duration(seconds-10) * time.Second
	// The first attempts happen at once. Waiting for them must not move the
	// clock, or the wait the peer asked for would start before the check.
	for _, origin := range domains {
		err := pollReal(ctx, 30*time.Second, func() error {
			got, err := attempts(origin)
			if err != nil {
				return err
			}
			if len(got) == 0 {
				return fmt.Errorf("%s has sent nothing to %s", origin, peer)
			}
			for id, n := range got {
				if n < 1 {
					return fmt.Errorf("%s has not tried envelope %d to %s yet", origin, id, peer)
				}
			}
			before[origin] = got
			return nil
		})
		if err != nil {
			return err
		}
	}
	for elapsed() < limit {
		if passTime != nil && elapsed()+5*time.Second < limit {
			if err := passTime(ctx, 5*time.Second); err != nil {
				return err
			}
			moved += 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		for _, origin := range domains {
			got, err := attempts(origin)
			if err != nil {
				return err
			}
			for id, n := range got {
				if n != before[origin][id] {
					return fmt.Errorf("%s tried envelope %d to %s again after %d of the %d seconds it was asked to wait",
						origin, id, peer, int(elapsed().Seconds()), seconds)
				}
			}
		}
	}
	return nil
}

func toSeconds(n int, unit string) int {
	if strings.HasPrefix(unit, "minute") {
		return n * 60
	}
	return n
}

// membersSee waits until each listed person, or every member for
// "everyone", has the thread with that many messages, and optionally until
// they all read it in the same order.
func (w *world) membersSee(ctx context.Context, seconds int, list string, count int, subject string, sameOrder bool) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	people := c.members
	if list != "everyone" {
		if people, err = w.persons(list); err != nil {
			return err
		}
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		var first []string
		for _, p := range people {
			got, err := w.transcript(ctx, p, c)
			if err == nil && len(got) != count {
				err = fmt.Errorf("%s sees %d messages on %q, want %d", p.name, len(got), subject, count)
			}
			if err == nil && sameOrder {
				if first == nil {
					first = got
				} else if !slices.Equal(got, first) {
					err = fmt.Errorf("%s reads:\n  %s\nwhile %s reads:\n  %s", p.name,
						strings.Join(got, "\n  "), people[0].name, strings.Join(first, "\n  "))
				}
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// sendsConfirmed checks every send since the last such check: the user is
// never kept waiting on a peer, however many are down (RFC 3.1).
func (w *world) sendsConfirmed(seconds int) error {
	limit := time.Duration(seconds) * time.Second
	var errs []error
	for i, took := range w.sends {
		if took > limit {
			errs = append(errs, fmt.Errorf("send %d took %s, more than %s", i+1, took.Round(time.Millisecond), limit))
		}
	}
	w.sends = nil
	return errors.Join(errs...)
}

// notDelayed checks that the story's envelopes between two Domains that
// stayed up were each delivered on the first attempt, so a third Domain
// being down held nothing up between them (RFC 3.1, fan-out is independent).
func (w *world) notDelayed(ctx context.Context, a, b string) error {
	var errs []error
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		all, err := w.stack.envelopes(ctx, pair[0])
		if err != nil {
			return err
		}
		for _, e := range all {
			if e.PeerDomain != pair[1] || !w.ours(e.Recipients...) {
				continue
			}
			if e.Status != "delivered" || e.Attempts != 1 {
				errs = append(errs, fmt.Errorf("envelope %d from %s to %s is %s after %d attempts",
					e.ID, pair[0], pair[1], e.Status, e.Attempts))
			}
		}
	}
	return errors.Join(errs...)
}

// stillSee checks a count that has not moved. For someone whose Domain is
// down, there is nothing to read and nothing can have reached them: their
// Domain answering nothing is the check.
func (w *world) stillSee(ctx context.Context, list string, count int) error {
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
		if slices.Contains(w.downed, p.domain) {
			if w.stack.health(ctx, p.domain) == nil {
				errs = append(errs, fmt.Errorf("%s should be down, but answers", p.domain))
			}
			continue
		}
		got, err := w.transcript(ctx, p, c)
		if err == nil && len(got) != count {
			err = fmt.Errorf("%s sees %d messages, want still %d", p.name, len(got), count)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// replyAfter checks that one person's last reply comes after two others' in
// every member's copy of the thread.
func (w *world) replyAfter(ctx context.Context, last, first, second string) error {
	bodies := map[string]string{}
	for _, name := range []string{last, first, second} {
		p, err := w.person(name)
		if err != nil {
			return err
		}
		body, ok := w.replyBy[name]
		if !ok {
			return fmt.Errorf("%s has not replied in this story", name)
		}
		bodies[name] = p.address + ": " + body
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range c.members {
		got, err := w.transcript(ctx, p, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		at := slices.Index(got, bodies[last])
		for _, earlier := range []string{first, second} {
			if i := slices.Index(got, bodies[earlier]); at < 0 || i < 0 || i > at {
				errs = append(errs, fmt.Errorf("%s reads %s's reply before %s's:\n  %s",
					p.name, last, earlier, strings.Join(got, "\n  ")))
			}
		}
	}
	return errors.Join(errs...)
}
