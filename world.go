package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// person is one name from a story, with a fresh account on their Domain.
type person struct {
	name    string
	domain  string
	address string
	session *Session
}

// conversation is a thread started during the story, as the steps know it:
// who started it, who has been put on it since, and who wrote in it.
type conversation struct {
	author  *person
	subject string
	members []*person
	writers []*person
}

// world is the state of one running story. Every story gets a new one, with
// new accounts, so stories share nothing and run in any order.
type world struct {
	stack         *Stack
	run           string
	people        map[string]*person
	conversations []*conversation

	// lastSend is how long the most recent "writes to" took to be accepted.
	lastSend struct {
		author string
		took   time.Duration
	}
	// lastSubject is the thread a following "it" refers to.
	lastSubject string
	// firstEmail is the newest email in the mail sink when the story began,
	// so email checks see only what the story caused.
	firstEmail    int
	firstEmailErr error
}

func newWorld(stack *Stack) *world {
	buf := make([]byte, 4)
	rand.Read(buf)
	return &world{stack: stack, run: hex.EncodeToString(buf), people: map[string]*person{}}
}

// canonical is the address a story writes for a person, such as
// alice@103mail.com.
func (p *person) canonical() string {
	return strings.ToLower(p.name) + "@" + p.domain
}

// addressFor gives a person a unique address for this run, so that the same
// story run twice, or two stories at once, never meet in one mailbox.
func (w *world) addressFor(name, domain string) string {
	return strings.ToLower(name) + "." + w.run + "@" + domain
}

// rewrite replaces every canonical address in a story's text with the
// address used in this run.
func (w *world) rewrite(s string) string {
	for _, p := range w.people {
		s = strings.ReplaceAll(s, p.canonical(), p.address)
	}
	return s
}

// names splits "Ann, Bob, Carol and Erin" into its names.
func names(list string) []string {
	var out []string
	for _, part := range strings.Split(strings.ReplaceAll(list, " and ", ", "), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (w *world) person(name string) (*person, error) {
	p, ok := w.people[name]
	if !ok {
		return nil, fmt.Errorf("%s has no account in this story", name)
	}
	return p, nil
}

func (w *world) persons(list string) ([]*person, error) {
	var out []*person
	for _, n := range names(list) {
		p, err := w.person(n)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// everyone is every person in the story, in a stable order.
func (w *world) everyone() []*person {
	out := make([]*person, 0, len(w.people))
	for _, p := range w.people {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *person) int { return strings.Compare(a.name, b.name) })
	return out
}

// conversation finds the thread a step means. owner is the person whose
// thread it is ("Alice's"), or empty when the subject alone is enough.
func (w *world) conversation(owner, subject string) (*conversation, error) {
	var found []*conversation
	for _, c := range w.conversations {
		if c.subject == subject && (owner == "" || c.author.name == owner) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		if owner != "" {
			return nil, fmt.Errorf("%s did not start a thread %q in this story", owner, subject)
		}
		return nil, fmt.Errorf("no thread %q was started in this story", subject)
	case 1:
		return found[0], nil
	}
	return nil, fmt.Errorf("%d threads are called %q; the step has to say whose", len(found), subject)
}

func (c *conversation) has(p *person) bool {
	return slices.Contains(c.members, p)
}

// wrote records that p wrote a message in the thread.
func (c *conversation) wrote(p *person) {
	if !slices.Contains(c.writers, p) {
		c.writers = append(c.writers, p)
	}
}

// located is a thread as one person sees it, with the folder it is in.
type located struct {
	folder string
	thread
}

// view lists every thread in every folder of every mailbox the person has.
func (p *person) view(ctx context.Context) ([]located, error) {
	mailboxes, err := p.session.mailboxes(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	var out []located
	for _, m := range mailboxes {
		folders, err := p.session.folders(ctx, m.MailboxID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.name, err)
		}
		for _, f := range folders {
			threads, err := p.session.threads(ctx, f.FolderID)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.name, err)
			}
			for _, t := range threads {
				out = append(out, located{folder: f.Name, thread: t})
			}
		}
	}
	return out, nil
}

// called returns the threads in a person's view with a subject.
func (p *person) called(ctx context.Context, subject string) ([]located, error) {
	view, err := p.view(ctx)
	if err != nil {
		return nil, err
	}
	var out []located
	for _, t := range view {
		if t.Subject == subject {
			out = append(out, t)
		}
	}
	return out, nil
}

// members is everyone on a thread as the person sees it. The API leaves the
// viewer out of the subscriber list, so they are added back.
func (p *person) members(t located) []string {
	out := []string{p.address}
	for _, s := range t.Subscribers {
		out = append(out, strings.ToLower(s.Email))
	}
	return out
}

// find returns the person's own copy of a conversation: the thread with its
// subject whose members include everyone on the conversation. Two threads
// with the same subject but different people stay apart this way.
func (p *person) find(ctx context.Context, c *conversation) (located, error) {
	threads, err := p.called(ctx, c.subject)
	if err != nil {
		return located{}, err
	}
	var found []located
	for _, t := range threads {
		members := p.members(t)
		all := true
		for _, m := range c.members {
			all = all && slices.Contains(members, m.address)
		}
		if all {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return located{}, fmt.Errorf("%s has no thread %q", p.name, c.subject)
	case 1:
		return found[0], nil
	}
	return located{}, fmt.Errorf("%s has %d copies of the thread %q", p.name, len(found), c.subject)
}

// defaultFolder is the folder a new thread is written in: the default folder
// of the default mailbox, which is the Inbox.
func (p *person) defaultFolder(ctx context.Context) (int64, error) {
	mailboxes, err := p.session.mailboxes(ctx)
	if err != nil {
		return 0, err
	}
	for _, m := range mailboxes {
		if !m.IsDefault {
			continue
		}
		folders, err := p.session.folders(ctx, m.MailboxID)
		if err != nil {
			return 0, err
		}
		for _, f := range folders {
			if f.IsDefault {
				return f.FolderID, nil
			}
		}
	}
	return 0, fmt.Errorf("%s has no default folder to write in", p.name)
}

// eventually runs check until it passes or the deadline is reached, and then
// reports what was still wrong. Nothing in the suite sleeps for a fixed time.
func eventually(ctx context.Context, seconds int, check func() error) error {
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		err := check()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("still not so after %d seconds:\n%w", seconds, err)
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
