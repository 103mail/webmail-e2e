package e2e

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// registerAttachments binds the phrases of the attachment story (03): files
// that cross Domains, their limits, and how a Domain stores them.
func (w *world) registerAttachments(sc *godog.ScenarioContext) {
	sc.Step(`^(\w+) writes to (.+?): "([^"]*)" attaching (.+)$`, w.writesAttaching)
	sc.Step(`^(\w+) wrote to (.+?): "([^"]*)" attaching (.+)$`, w.wroteAttaching)
	sc.Step(`^(\w+) replies to "([^"]*)" attaching (.+)$`, w.repliesAttaching)
	sc.Step(`^(\w+) is invited to "([^"]*)"$`, w.isInvited)
	sc.Step(`^within (\d+) seconds (.+?)(?: each)? sees? (\d+) attachments on "([^"]*)"$`, w.seeAttachments)
	sc.Step(`^every recipient's copy of each file is byte-identical to (\w+)'s$`, w.copiesIdentical)
	sc.Step(`^within (\d+) seconds (.+?) each see ("[^"]+"(?:(?:, | and )"[^"]+")+)$`, w.eachSeeFiles)
	sc.Step(`^(\w+) sees "([^"]+)" and (\w+) sees "([^"]+)", each byte-identical$`, w.twoSeeFiles)
	sc.Step(`^within (\d+) seconds (\w+) can download all (\d+) files, each byte-identical$`, w.downloadAll)
	sc.Step(`^no email has left any Domain with an attachment$`, w.noEmailWithAttachment)
	sc.Step(`^within (\d+) seconds (\w+) sees "([^"]+)" on both threads$`, w.seesFileOnBoth)
	sc.Step(`^(\S+) stores the bytes of "([^"]+)" once$`, w.storesOnce)
	sc.Step(`^(\w+) is told the attachment is larger than the (\d+) MB limit$`, w.toldTooLarge)
	sc.Step(`^nothing is delivered to (.+)$`, w.nothingDelivered)
	sc.Step(`^(\S+)'s attachment limit is (\d+) MB$`, w.attachmentLimit)
	sc.Step(`^within (\d+) seconds (\w+) has "([^"]*)" with "([^"]*)"$`, w.hasWithFile)
	sc.Step(`^(.+?) each have exactly one thread with an attachment: (\w+)'s "([^"]*)"$`, w.oneThreadWithFiles)
}

// fileSpec reads one file of an "attaching" list: `"tender.pdf" (8 MB)`, or
// `the same "logo.png"` for a file attached earlier in the story.
var fileSpec = regexp.MustCompile(`(the same )?"([^"]+)"(?: \((\d+) (KB|MB)\))?`)

// filesFrom turns an "attaching" list into uploads. A new file gets bytes no
// other story run has, so a Domain cannot already hold them.
func (w *world) filesFrom(list string) ([]upload, error) {
	var out []upload
	for _, m := range fileSpec.FindAllStringSubmatch(list, -1) {
		name := m[2]
		if m[1] != "" || m[3] == "" {
			data, ok := w.files[name]
			if !ok {
				return nil, fmt.Errorf("no file %q was attached earlier in this story", name)
			}
			out = append(out, upload{FileName: name, MimeType: mimeType(name), FileData: data})
			continue
		}
		n, _ := strconv.Atoi(m[3])
		size := n << 10
		if m[4] == "MB" {
			size = n << 20
		}
		data := w.fileBytes(name, size)
		w.files[name] = data
		out = append(out, upload{FileName: name, MimeType: mimeType(name), FileData: data})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no files in %q", list)
	}
	return out, nil
}

// fileBytes is size bytes that only this run and this file name produce.
func (w *world) fileBytes(name string, size int) []byte {
	out := make([]byte, 0, size+sha256.Size)
	block := sha256.Sum256([]byte(w.run + "/" + name))
	for len(out) < size {
		out = append(out, block[:]...)
		block = sha256.Sum256(block[:])
	}
	return out[:size]
}

func mimeType(name string) string {
	switch path.Ext(name) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".mov":
		return "video/quicktime"
	}
	return "application/octet-stream"
}

// writesAttaching starts a thread with files. A send the Domain refuses as
// too large is recorded rather than failed, for the step that says so.
func (w *world) writesAttaching(ctx context.Context, author, list, subject, files string) error {
	uploads, err := w.filesFrom(files)
	if err != nil {
		return err
	}
	err = w.writeWith(ctx, author, list, subject, "Files attached.", uploads...)
	var refused *apiError
	if errors.As(err, &refused) && refused.status == 413 {
		w.lastAnswer = answer{status: refused.status, code: refused.code, detail: refused.detail}
		w.lastSubject = subject
		return nil
	}
	if err != nil {
		return err
	}
	for _, u := range uploads {
		w.attachedTo[u.FileName] = append(w.attachedTo[u.FileName], subject)
	}
	return nil
}

// wroteAttaching is writesAttaching as a Given: it waits until everyone
// written to has the thread.
func (w *world) wroteAttaching(ctx context.Context, author, list, subject, files string) error {
	if err := w.writesAttaching(ctx, author, list, subject, files); err != nil {
		return err
	}
	c, err := w.conversation(author, subject)
	if err != nil {
		return err
	}
	return w.untilEveryoneHas(ctx, c, "")
}

func (w *world) repliesAttaching(ctx context.Context, who, subject, files string) error {
	uploads, err := w.filesFrom(files)
	if err != nil {
		return err
	}
	if err := w.replyWith(ctx, who, "", subject, "Files attached.", uploads...); err != nil {
		return err
	}
	for _, u := range uploads {
		w.attachedTo[u.FileName] = append(w.attachedTo[u.FileName], subject)
	}
	return nil
}

// isInvited is an invitation from whoever started the thread.
func (w *world) isInvited(ctx context.Context, who, subject string) error {
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	return w.invites(ctx, c.author.name, who, "", subject)
}

// filesOn lists every attachment on a person's copy of a thread, by name.
func (w *world) filesOn(ctx context.Context, p *person, c *conversation) (map[string][]attachmentRef, error) {
	t, err := p.find(ctx, c)
	if err != nil {
		return nil, err
	}
	messages, err := p.session.messages(ctx, t.ThreadID)
	if err != nil {
		return nil, err
	}
	out := map[string][]attachmentRef{}
	for _, m := range messages {
		for _, a := range m.Attachments {
			out[a.FileName] = append(out[a.FileName], a)
		}
	}
	return out, nil
}

// identical downloads a person's copies of a file and compares each with the
// bytes that were attached.
func (w *world) identical(ctx context.Context, p *person, name string, copies []attachmentRef) error {
	want, ok := w.files[name]
	if !ok {
		return fmt.Errorf("no file %q was attached in this story", name)
	}
	if len(copies) == 0 {
		return fmt.Errorf("%s has no %q", p.name, name)
	}
	for _, a := range copies {
		got, err := p.session.download(ctx, a.AttachmentID)
		if err != nil {
			return fmt.Errorf("%s downloads %q: %w", p.name, name, err)
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("%s's %q is %d bytes and differs from the %d bytes attached", p.name, name, len(got), len(want))
		}
	}
	return nil
}

func (w *world) seeAttachments(ctx context.Context, seconds int, list string, count int, subject string) error {
	c, err := w.conversation("", subject)
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
			files, err := w.filesOn(ctx, p, c)
			n := 0
			for _, copies := range files {
				n += len(copies)
			}
			if err == nil && n != count {
				err = fmt.Errorf("%s sees %d attachments on %q, want %d", p.name, n, subject, count)
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// copiesIdentical checks every member other than the author against the
// bytes of every file on the thread.
func (w *world) copiesIdentical(ctx context.Context, author string) error {
	c, err := w.conversation(author, w.lastSubject)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range c.members {
		if p == c.author {
			continue
		}
		files, err := w.filesOn(ctx, p, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for name, copies := range files {
			errs = append(errs, w.identical(ctx, p, name, copies))
		}
	}
	return errors.Join(errs...)
}

var quoted = regexp.MustCompile(`"([^"]+)"`)

func (w *world) eachSeeFiles(ctx context.Context, seconds int, list, names string) error {
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
			files, err := w.filesOn(ctx, p, c)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, m := range quoted.FindAllStringSubmatch(names, -1) {
				if len(files[m[1]]) == 0 {
					errs = append(errs, fmt.Errorf("%s does not see %q", p.name, m[1]))
				}
			}
		}
		return errors.Join(errs...)
	})
}

// twoSeeFiles follows a "within N seconds" step and shares its deadline:
// each reply travels to every Domain in an envelope of its own, so the
// senders' copies can arrive a worker tick after the ones that step saw.
func (w *world) twoSeeFiles(ctx context.Context, first, firstFile, second, secondFile string) error {
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	return eventually(ctx, 60, func() error {
		var errs []error
		for _, pair := range [][2]string{{first, firstFile}, {second, secondFile}} {
			p, err := w.person(pair[0])
			if err != nil {
				return err
			}
			files, err := w.filesOn(ctx, p, c)
			if err == nil {
				err = w.identical(ctx, p, pair[1], files[pair[1]])
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// downloadAll checks that a person has every file attached to the thread,
// each byte-identical, which is what an invitation that brings the history
// means for files (RFC 5.6).
func (w *world) downloadAll(ctx context.Context, seconds int, who string, count int) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation("", w.lastSubject)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		files, err := w.filesOn(ctx, p, c)
		if err != nil {
			return err
		}
		if len(files) != count {
			return fmt.Errorf("%s has %d files on %q, want %d", p.name, len(files), c.subject, count)
		}
		var errs []error
		for name, copies := range files {
			errs = append(errs, w.identical(ctx, p, name, copies))
		}
		return errors.Join(errs...)
	})
}

// noEmailWithAttachment checks every email any Domain sent during the story
// for an attachment, or for the name of any file the story attached.
func (w *world) noEmailWithAttachment(ctx context.Context) error {
	if w.firstEmailErr != nil {
		return w.firstEmailErr
	}
	emails, err := w.stack.emailsAfter(ctx, w.firstEmail)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range emails {
		raw := string(e.Notification.Raw)
		if strings.Contains(strings.ToLower(raw), "content-disposition: attachment") {
			errs = append(errs, fmt.Errorf("email %d to %v carries an attachment", e.ID, e.Notification.To))
		}
		for name := range w.files {
			for _, part := range []string{raw, e.Notification.Text, e.Notification.HTML, e.Notification.Subject} {
				if strings.Contains(part, name) {
					errs = append(errs, fmt.Errorf("email %d to %v names %q", e.ID, e.Notification.To, name))
					break
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (w *world) seesFileOnBoth(ctx context.Context, seconds int, who, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	subjects := w.attachedTo[name]
	if len(subjects) != 2 {
		return fmt.Errorf("%q was attached to %d threads, not two", name, len(subjects))
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, subject := range subjects {
			c, err := w.conversation("", subject)
			if err != nil {
				return err
			}
			files, err := w.filesOn(ctx, p, c)
			if err == nil {
				err = w.identical(ctx, p, name, files[name])
			}
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

// storesOnce checks that a Domain holds a file's bytes once, however many
// attachments refer to them.
func (w *world) storesOnce(ctx context.Context, domain, name string) error {
	data, ok := w.files[name]
	if !ok {
		return fmt.Errorf("no file %q was attached in this story", name)
	}
	sum := md5.Sum(data)
	stored, err := w.stack.storedFiles(ctx, domain, hex.EncodeToString(sum[:]))
	if err != nil {
		return err
	}
	if len(stored) != 1 || !stored[0].Stored {
		return fmt.Errorf("%s holds the bytes of %q as %+v, want one stored file", domain, name, stored)
	}
	if want := int64(len(w.attachedTo[name])); stored[0].Attachments < want {
		return fmt.Errorf("%s refers to the bytes of %q from %d attachments, want at least %d",
			domain, name, stored[0].Attachments, want)
	}
	return nil
}

func (w *world) toldTooLarge(who string, limit int) error {
	if w.lastSend.author != who && w.lastAnswer.status == 0 {
		return fmt.Errorf("%s made no send that was refused", who)
	}
	a := w.lastAnswer
	if want := fmt.Sprintf("larger than the %d MB limit", limit); a.status != 413 || a.code != "too_large" ||
		!strings.Contains(a.detail, want) {
		return fmt.Errorf("%s was told %s, want 413 too_large saying %q", who, a, want)
	}
	return nil
}

// nothingDelivered checks that a refused send left no trace: no envelope to
// those Domains and no thread for anyone on them.
func (w *world) nothingDelivered(ctx context.Context, list string) error {
	domains := names(strings.ReplaceAll(list, " or ", " and "))
	var errs []error
	for _, origin := range w.domains() {
		all, err := w.stack.envelopes(ctx, origin)
		if err != nil {
			return err
		}
		for _, e := range all {
			if w.ours(e.Recipients...) && slices.Contains(domains, e.PeerDomain) {
				errs = append(errs, fmt.Errorf("%s sent envelope %d to %s", origin, e.ID, e.PeerDomain))
			}
		}
	}
	for _, p := range w.everyone() {
		if slices.Contains(domains, p.domain) {
			errs = append(errs, w.checkThreadCount(ctx, p, w.lastSubject, "0"))
		}
	}
	return errors.Join(errs...)
}

// domains lists the Domains of everyone in the story.
func (w *world) domains() []string {
	var out []string
	for _, p := range w.everyone() {
		if !slices.Contains(out, p.domain) {
			out = append(out, p.domain)
		}
	}
	return out
}

func (w *world) attachmentLimit(ctx context.Context, domain string, mb int) error {
	cfg, err := w.stack.federationConfig(domain)
	if err != nil {
		return err
	}
	cfg["maxAttachmentBytes"] = mb << 20
	return w.reconfigure(ctx, domain, cfg)
}

func (w *world) hasWithFile(ctx context.Context, seconds int, who, subject, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	c, err := w.conversation("", subject)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		files, err := w.filesOn(ctx, p, c)
		if err != nil {
			return err
		}
		return w.identical(ctx, p, name, files[name])
	})
}

// oneThreadWithFiles checks that, of all a person's threads, exactly one
// carries attachments, and it is the one named.
func (w *world) oneThreadWithFiles(ctx context.Context, list, owner, subject string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := w.conversation(owner, subject)
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
		own, err := p.find(ctx, c)
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
			files := slices.ContainsFunc(messages, func(m message) bool { return len(m.Attachments) > 0 })
			if files && t.ThreadID != own.ThreadID {
				errs = append(errs, fmt.Errorf("%s has attachments on %q too", p.name, t.Subject))
			}
			if !files && t.ThreadID == own.ThreadID {
				errs = append(errs, fmt.Errorf("%s has no attachment on %q", p.name, t.Subject))
			}
		}
	}
	return errors.Join(errs...)
}
