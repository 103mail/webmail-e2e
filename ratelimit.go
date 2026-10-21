package e2e

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// registerRateLimit binds the phrases for a Domain that caps how fast one
// peer may send, without slowing down the others (story 08, RFC 9.10).
func (w *world) registerRateLimit(sc *godog.ScenarioContext) {
	sc.Step(`^(\S+) accepts at most (\d+) envelopes per minute from one peer$`, w.acceptsAtMost)
	sc.Step(`^the test peer sends (\w+) (\d+) messages within a minute$`, w.peerFloods)
	sc.Step(`^(\w+) writes to (.+?): "([^"]*)" at the same time$`,
		func(ctx context.Context, author, list, subject string) error {
			return w.writes(ctx, author, list, subject, "")
		})
	sc.Step(`^the test peer gets (\d+) accepted and the rest 429 "([^"]*)" with a retry time$`, w.peerWasLimited)
	sc.Step(`^(\w+) has exactly (\d+) messages from the test peer$`, w.messagesFromPeer)
}

func (w *world) acceptsAtMost(ctx context.Context, domain string, perMinute int) error {
	cfg, err := w.stack.federationConfig(domain)
	if err != nil {
		return err
	}
	cfg["rateLimit"] = map[string]any{"perMinute": perMinute}
	return w.reconfigure(ctx, domain, cfg)
}

// peerFloods has the test peer send a person many messages on one thread,
// one envelope each, as fast as it can.
func (w *world) peerFloods(ctx context.Context, who string, count int) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	peer, err := w.stack.sender(testPeer)
	if err != nil {
		return err
	}
	from := "noisy." + w.run + "@" + testPeer
	threadID := newUUID()
	w.peerAnswers = nil
	for i := 1; i <= count; i++ {
		e := peer.envelope(from, threadID, "Noise", fmt.Sprintf("Noise %d", i), []string{p.address})
		r, err := peer.request(e)
		if err != nil {
			return err
		}
		a, err := peer.send(ctx, p.domain, r)
		if err != nil {
			return err
		}
		w.peerAnswers = append(w.peerAnswers, a)
	}
	return nil
}

func (w *world) peerWasLimited(accepted int, code string) error {
	var errs []error
	got := 0
	for i, a := range w.peerAnswers {
		switch {
		case a.status == 202:
			got++
		case a.status != 429 || a.code != code:
			errs = append(errs, fmt.Errorf("message %d was answered %s, want 202 or 429 %q", i+1, a, code))
		default:
			if seconds, err := strconv.Atoi(a.retryAfter); err != nil || seconds <= 0 {
				errs = append(errs, fmt.Errorf("message %d was limited without a retry time: Retry-After %q", i+1, a.retryAfter))
			}
		}
	}
	if got != accepted {
		errs = append(errs, fmt.Errorf("%d of %d messages were accepted, want %d", got, len(w.peerAnswers), accepted))
	}
	return errors.Join(errs...)
}

// messagesFromPeer counts the messages in a person's mailbox that came from
// the test peer's Domain.
func (w *world) messagesFromPeer(ctx context.Context, who string, want int) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	view, err := p.view(ctx)
	if err != nil {
		return err
	}
	got := 0
	for _, t := range view {
		messages, err := p.session.messages(ctx, t.ThreadID)
		if err != nil {
			return err
		}
		for _, m := range messages {
			if strings.HasSuffix(strings.ToLower(m.SenderEmail), "@"+testPeer) {
				got++
			}
		}
	}
	if got != want {
		return fmt.Errorf("%s has %d messages from the test peer, want %d", p.name, got, want)
	}
	return nil
}
