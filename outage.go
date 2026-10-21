package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The outage controls act on the test stack, not on any server: a Domain is
// taken down by stopping its containers, and its retry budget is set by
// recreating its worker. Containers carry a webmail.domain label, which is
// how a Domain's are found (docker-compose.yml in webmail-deploy).

// containers lists the ids of a Domain's containers, running or not.
func (s *Stack) containers(ctx context.Context, domain string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-aq",
		"--filter", "label=com.docker.compose.project=webmail",
		"--filter", "label=webmail.domain="+domain).Output()
	if err != nil {
		return nil, fmt.Errorf("list the containers of %s: %w", domain, err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, fmt.Errorf("the stack has no containers for %s", domain)
	}
	return ids, nil
}

// down stops every container of a Domain, so it answers nothing at all.
func (s *Stack) down(ctx context.Context, domain string) error {
	ids, err := s.containers(ctx, domain)
	if err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"stop", "-t", "2"}, ids...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("take %s down: %w: %s", domain, err, out)
	}
	return nil
}

// up starts a Domain's containers again and waits until it answers.
func (s *Stack) up(ctx context.Context, domain string) error {
	ids, err := s.containers(ctx, domain)
	if err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"start"}, ids...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("bring %s back: %w: %s", domain, err, out)
	}
	return pollReal(ctx, time.Minute, func() error { return s.health(ctx, domain) })
}

// passTime moves the clock every Domain shares forward by d: the offset in
// clock/offset in webmail-deploy, which each API and worker adds to the real time
// (WEBMAIL_CLOCK_FILE). It is replaced whole, so a server never reads half a
// file, and it only ever moves forward.
func (s *Stack) passTime(ctx context.Context, d time.Duration) error {
	dir := filepath.Join(s.deployDir, "clock")
	path := filepath.Join(dir, "offset")
	var offset time.Duration
	if raw, err := os.ReadFile(path); err == nil {
		if offset, err = time.ParseDuration(strings.TrimSpace(string(raw))); err != nil {
			return fmt.Errorf("read the stack's clock offset: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read the stack's clock offset: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "offset-*")
	if err != nil {
		return fmt.Errorf("move the stack's clock (run ./up.sh first): %w", err)
	}
	if _, err := tmp.WriteString((offset + d).String() + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Readable by the servers, which run as another user.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// budgetVariable is the compose variable that sets a Domain's retry budget.
func budgetVariable(domain string) string {
	return "RETRY_BUDGET_" + regexp.MustCompile(`[^A-Z0-9]`).ReplaceAllString(strings.ToUpper(domain), "_")
}

// setRetryBudget recreates a Domain's worker with a retry budget, or with
// the RFC's 72 hours when budget is empty.
func (s *Stack) setRetryBudget(ctx context.Context, domain, budget string) error {
	ids, err := s.containers(ctx, domain)
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, "docker", append([]string{"inspect", "--format",
		`{{index .Config.Labels "com.docker.compose.service"}}`}, ids...)...).Output()
	if err != nil {
		return fmt.Errorf("find the worker of %s: %w", domain, err)
	}
	var worker string
	for _, service := range strings.Fields(string(out)) {
		if strings.HasPrefix(service, "worker-") {
			worker = service
		}
	}
	if worker == "" {
		return fmt.Errorf("%s has no worker", domain)
	}
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", filepath.Join(s.deployDir, "docker-compose.yml"),
		"up", "-d", "--no-deps", "--force-recreate", worker)
	cmd.Env = append(os.Environ(), budgetVariable(domain)+"="+budget)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("recreate %s with a retry budget of %q: %w: %s", worker, budget, err, out)
	}
	return nil
}

// registerOutage binds the outage phrases, and puts the stack back after
// every story, so one story's outage never reaches the next.
func (w *world) registerOutage(sc *godog.ScenarioContext) {
	sc.Step(`^the retry budget on (\S+) is (\d+) minutes?$`, w.retryBudget)
	sc.Step(`^(\S+) goes down(?: for good)?$`, w.goesDown)
	sc.Step(`^(\S+) comes back$`, w.comesBack)
	sc.Step(`^within (\d+) seconds (\w+) has "([^"]*)"$`, w.personHas)
	sc.Step(`^within (\d+) minutes (\w+) has "([^"]*)" naming (.+) and the reason "([^"]*)"$`, w.bouncedFor)
	sc.Step(`^the bounce says it will be sent again$`, w.bounceSaysResend)
	sc.Step(`^(\S+) makes no further delivery attempt for "([^"]*)"$`, w.noFurtherAttempt)
	sc.Step(`^(\w+)'s own Domain retries to (\S+) on its own budget, independent of (\S+)$`, w.retriesOnItsOwn)
	sc.Step(`^within (\d+) minutes ([\w ,]+?) sees? "([^"]*)"$`,
		func(ctx context.Context, minutes int, list, body string) error {
			return w.eventuallySee(ctx, minutes*60, list, body)
		})
	sc.Step(`^nobody has a duplicate message$`, w.noDuplicates)

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		var errs []error
		for _, domain := range w.downed {
			errs = append(errs, w.stack.up(ctx, domain))
		}
		for _, domain := range w.budgets {
			errs = append(errs, w.stack.setRetryBudget(ctx, domain, ""))
		}
		w.downed, w.budgets = nil, nil
		return ctx, errors.Join(errs...)
	})
}

func (w *world) retryBudget(ctx context.Context, domain string, minutes int) error {
	if err := w.stack.setRetryBudget(ctx, domain, fmt.Sprintf("%dm", minutes)); err != nil {
		return err
	}
	w.budgets = append(w.budgets, domain)
	return nil
}

func (w *world) goesDown(ctx context.Context, domain string) error {
	if err := w.stack.down(ctx, domain); err != nil {
		return err
	}
	if !slices.Contains(w.downed, domain) {
		w.downed = append(w.downed, domain)
	}
	return nil
}

func (w *world) comesBack(ctx context.Context, domain string) error {
	if err := w.stack.up(ctx, domain); err != nil {
		return err
	}
	w.downed = slices.DeleteFunc(w.downed, func(d string) bool { return d == domain })
	return nil
}

func (w *world) personHas(ctx context.Context, seconds int, who, subject string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		threads, err := p.called(ctx, subject)
		if err == nil && len(threads) == 0 {
			err = fmt.Errorf("%s has no %q", p.name, subject)
		}
		return err
	})
}

// bouncedFor checks for a bounce naming every listed person and the reason.
func (w *world) bouncedFor(ctx context.Context, minutes int, who, subject, list, reason string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	named, err := w.persons(list)
	if err != nil {
		return err
	}
	return eventually(ctx, minutes*60, func() error {
		got, err := p.bounces(ctx, subject)
		if err != nil {
			return err
		}
		for _, b := range got {
			all := !slices.ContainsFunc(named, func(n *person) bool { return !b.names(n.address) })
			if all && strings.Contains(b.body, "Reason: "+reason) {
				w.lastBounce = b
				return nil
			}
		}
		return fmt.Errorf("%s has no bounce %q naming %s for %s", p.name, subject, list, reason)
	})
}

func (w *world) bounceSaysResend() error {
	if !strings.Contains(w.lastBounce.body, "will be sent again") {
		return fmt.Errorf("the bounce does not say the message will be sent again:\n%s", w.lastBounce.body)
	}
	return nil
}

// noFurtherAttempt watches a Domain's envelopes for this story while ten
// minutes pass on the stack's clock: none may be attempted again or
// re-queued while the peer stays away.
func (w *world) noFurtherAttempt(ctx context.Context, domain, subject string) error {
	snapshot := func() (string, error) {
		all, err := w.stack.envelopes(ctx, domain)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, e := range all {
			if w.ours(e.Recipients...) {
				lines = append(lines, fmt.Sprintf("%d %s %s", e.ID, e.PeerDomain, e.Status))
			}
		}
		return strings.Join(lines, "\n"), nil
	}
	before, err := snapshot()
	if err != nil {
		return err
	}
	if strings.Contains(before, "pending") {
		return fmt.Errorf("%s still has envelopes pending:\n%s", domain, before)
	}
	for passed := time.Duration(0); passed < 10*time.Minute; passed += 30 * time.Second {
		if passTime != nil {
			if err := passTime(ctx, 30*time.Second); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		after, err := snapshot()
		if err != nil {
			return err
		}
		if after != before {
			return fmt.Errorf("%s tried again:\nbefore\n%s\nafter\n%s", domain, before, after)
		}
	}
	return nil
}

// retriesOnItsOwn checks that a person's Domain is still retrying to a peer
// that another Domain has already given up on.
func (w *world) retriesOnItsOwn(ctx context.Context, who, peer, other string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	return eventually(ctx, 60, func() error {
		all, err := w.stack.envelopes(ctx, p.domain)
		if err != nil {
			return err
		}
		for _, e := range all {
			if e.PeerDomain == peer && w.ours(e.Recipients...) && e.Status == "pending" && e.Attempts > 0 {
				return nil
			}
		}
		return fmt.Errorf("%s is not retrying to %s", p.domain, peer)
	})
}

// noDuplicates checks every member's copy of the last thread for a message
// that appears twice.
func (w *world) noDuplicates(ctx context.Context) error {
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
		seen := map[string]bool{}
		for _, line := range got {
			if seen[line] {
				errs = append(errs, fmt.Errorf("%s has %q twice", p.name, line))
			}
			seen[line] = true
		}
	}
	return errors.Join(errs...)
}
