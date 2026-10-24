package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// The chat API as the steps use it, mirroring webmail-server's internal/chat
// JSON and keeping only the fields the steps look at. Every call is made by a
// person on their own Domain, so a chat is only ever seen the way that Domain
// shows it.

type chatParticipantRef struct {
	Email string `json:"email"`
}

type chatPreview struct {
	SenderEmail string `json:"senderEmail"`
	Body        string `json:"body"`
}

type chatRow struct {
	ChatID       int64                `json:"chatId"`
	Type         string               `json:"type"`
	Name         *string              `json:"name"`
	UnreadCount  int64                `json:"unreadCount"`
	Participants []chatParticipantRef `json:"participants"`
	LastMessage  *chatPreview         `json:"lastMessage"`
}

type chatMessage struct {
	MessageID      int64           `json:"messageId"`
	SenderEmail    string          `json:"senderEmail"`
	Body           string          `json:"body"`
	DeliveryStatus *deliveryStatus `json:"deliveryStatus"`
}

// deliveryStatus counts every member's copy of a message, the sender's
// included, which is read the moment it is sent.
type deliveryStatus struct {
	Sent      int `json:"sent"`
	Delivered int `json:"delivered"`
	Read      int `json:"read"`
}

type chatParticipant struct {
	ParticipantID int64  `json:"participantId"`
	Email         string `json:"email"`
}

// chatMailbox is the mailbox chats are listed and created in: the default one.
func (s *Session) chatMailbox(ctx context.Context) (int64, error) {
	mailboxes, err := s.mailboxes(ctx)
	if err != nil {
		return 0, err
	}
	for _, m := range mailboxes {
		if m.IsDefault {
			return m.MailboxID, nil
		}
	}
	return 0, fmt.Errorf("no default mailbox to chat in")
}

func (s *Session) chats(ctx context.Context) ([]chatRow, error) {
	mailboxID, err := s.chatMailbox(ctx)
	if err != nil {
		return nil, err
	}
	var out []chatRow
	err = s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/chat/chat/%d", mailboxID), nil, &out)
	return out, err
}

func (s *Session) addChat(ctx context.Context, chatType string, participants []string, name string) (int64, error) {
	mailboxID, err := s.chatMailbox(ctx)
	if err != nil {
		return 0, err
	}
	in := map[string]any{"type": chatType, "participants": participants}
	if name != "" {
		in["name"] = name
	}
	var id int64
	err = s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/chat/chat/%d", mailboxID), in, &id)
	return created("new chat", id, err)
}

func (s *Session) deleteChat(ctx context.Context, chatID int64) error {
	var ok bool
	if err := s.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/chat/chat/%d", chatID), nil, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deleting chat %d was refused", chatID)
	}
	return nil
}

// chatMessages lists a chat oldest first. The API answers newest first.
func (s *Session) chatMessages(ctx context.Context, chatID int64) ([]chatMessage, error) {
	var out []chatMessage
	if err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/chat/message/%d", chatID), nil, &out); err != nil {
		return nil, err
	}
	slices.Reverse(out)
	return out, nil
}

func (s *Session) addChatMessage(ctx context.Context, chatID int64, body string) (int64, error) {
	var id int64
	err := s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/chat/message/%d", chatID),
		map[string]any{"body": body, "attachments": []upload{}}, &id)
	return created("chat message", id, err)
}

// markChatRead is what opening a chat does: everything in it is read.
func (s *Session) markChatRead(ctx context.Context, chatID int64) error {
	var ok bool
	return s.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/chat/read/%d", chatID), map[string]any{}, &ok)
}

func (s *Session) chatParticipants(ctx context.Context, chatID int64) ([]chatParticipant, error) {
	var out []chatParticipant
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/chat/participant/%d", chatID), nil, &out)
	return out, err
}

func (s *Session) addChatParticipant(ctx context.Context, chatID int64, address string) (int64, error) {
	var id int64
	err := s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/chat/participant/%d", chatID),
		map[string]any{"email": address}, &id)
	return created("chat participant", id, err)
}

func (s *Session) removeChatParticipant(ctx context.Context, participantID int64) error {
	var ok bool
	if err := s.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/chat/participant/%d", participantID), nil, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("removing chat participant %d was refused", participantID)
	}
	return nil
}

// chatRoom is a chat started during the story, as the steps know it. A group
// chat is known by its name; a direct chat by its two people.
type chatRoom struct {
	name    string
	members []*person
}

func (r *chatRoom) String() string {
	if r.name != "" {
		return fmt.Sprintf("the group chat %q", r.name)
	}
	return fmt.Sprintf("the chat between %s and %s", r.members[0].name, r.members[1].name)
}

// registerChat binds the phrases of the chat stories (M7).
func (w *world) registerChat(sc *godog.ScenarioContext) {
	// Acts
	sc.Step(`^(\w+) starts a chat with (\w+): "([^"]*)"$`, w.startsChat)
	sc.Step(`^(\w+) started a chat with (\w+): "([^"]*)"$`, w.startedChat)
	sc.Step(`^(\w+) started the group chat "([^"]*)" with (.+)$`, w.startedGroupChat)
	sc.Step(`^(\w+) writes in (?:the chat with (\w+)|"([^"]*)"): "([^"]*)"$`, w.writesInChat)
	sc.Step(`^(\w+) wrote in "([^"]*)": "([^"]*)"$`, w.wroteInChat)
	sc.Step(`^(\w+) adds (\w+) to "([^"]*)"$`, w.addsToChat)
	sc.Step(`^(\w+) leaves the chat "([^"]*)"$`, w.leavesChat)
	sc.Step(`^(\w+) deletes the chat with (\w+)$`, w.deletesChat)
	sc.Step(`^(\w+) tries to write in "([^"]*)": "([^"]*)"$`, w.triesToWriteInChat)
	sc.Step(`^(.+) opens? the chat "([^"]*)"$`, w.opensChat)

	// Outcomes
	sc.Step(`^within (\d+) seconds (\w+) has a chat with (\w+) whose last message is "([^"]*)"$`, w.hasChatWithLast)
	sc.Step(`^(\w+) has (\d+) unread messages? in the chat with (\w+)$`, w.unreadInChat)
	sc.Step(`^(?:within (\d+) seconds )?the chat (?:between (\w+) and (\w+)|"([^"]*)") reads the same for (?:both|everyone):$`,
		w.chatReadsTheSame)
	sc.Step(`^(.+) (?:has|have) no chats$`, w.haveNoChats)
	sc.Step(`^within (\d+) seconds (.+?)(?: each)? (?:has|have) the group chat "([^"]*)" with (.+) as members$`,
		w.haveGroupChat)
	sc.Step(`^within (\d+) seconds every member of the chat "([^"]*)" sees (.+) as members$`, w.chatMembersAre)
	sc.Step(`^within (\d+) seconds (.+) see "([^"]*)" in "([^"]*)"$`, w.eventuallySeeInChat)
	// A name, capitalised, so that "nobody sees" below is not also read as one.
	sc.Step(`^([A-Z]\w*) (?:still )?sees "([^"]*)" in "([^"]*)"$`, w.seesInChat)
	sc.Step(`^(\w+) never sees "([^"]*)" in "([^"]*)"$`, w.neverSeesInChat)
	sc.Step(`^(\w+)'s message is refused because (?:he|she|they) (?:is|are) no longer in the chat$`,
		func() error { return w.refusedWith(http.StatusForbidden, "not_a_member") })
	sc.Step(`^nobody sees "([^"]*)" in "([^"]*)"$`, w.nobodySeesInChat)
	sc.Step(`^within (\d+) seconds (\w+) sees (?:her|his|their) message "([^"]*)" delivered to (\d+) and read by (\d+) of the (\d+) others$`,
		w.seesReceipts)
	sc.Step(`^(\w+) has no chat with (\w+)$`, w.hasNoChatWith)
	sc.Step(`^(\w+) still has the chat with (\w+), with (.+) as members$`, w.stillHasChatWith)
}

// ── Finding a chat ──────────────────────────────────────────────────────────

// direct is the chat room between two people, started in this story.
func (w *world) direct(a, b *person) (*chatRoom, error) {
	for _, r := range w.chats {
		if r.name == "" && slices.Contains(r.members, a) && slices.Contains(r.members, b) {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no chat between %s and %s was started in this story", a.name, b.name)
}

func (w *world) group(name string) (*chatRoom, error) {
	for _, r := range w.chats {
		if r.name == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no group chat %q was started in this story", name)
}

// room resolves "the chat with Bob" as one person says it, or a group chat by
// its name, whichever the step gave.
func (w *world) room(who *person, with, name string) (*chatRoom, error) {
	if name != "" {
		return w.group(name)
	}
	other, err := w.person(with)
	if err != nil {
		return nil, err
	}
	return w.direct(who, other)
}

// chatOf returns the person's own copy of a chat room: the group chat with its
// name, or the direct chat whose participants include the other person.
func (p *person) chatOf(ctx context.Context, r *chatRoom) (chatRow, error) {
	rows, err := p.session.chats(ctx)
	if err != nil {
		return chatRow{}, fmt.Errorf("%s: %w", p.name, err)
	}
	var found []chatRow
	for _, c := range rows {
		if r.name != "" {
			if c.Type == "group" && c.Name != nil && *c.Name == r.name {
				found = append(found, c)
			}
			continue
		}
		if c.Type != "direct" {
			continue
		}
		for _, m := range r.members {
			if m != p && c.has(m.address) {
				found = append(found, c)
			}
		}
	}
	switch len(found) {
	case 0:
		return chatRow{}, fmt.Errorf("%s does not have %s", p.name, r)
	case 1:
		return found[0], nil
	}
	return chatRow{}, fmt.Errorf("%s has %d copies of %s", p.name, len(found), r)
}

func (c chatRow) has(address string) bool {
	return slices.ContainsFunc(c.Participants, func(p chatParticipantRef) bool {
		return strings.EqualFold(p.Email, address)
	})
}

// members is everyone a person's copy of a chat lists, sorted.
func (c chatRow) members() []string {
	out := make([]string, len(c.Participants))
	for i, p := range c.Participants {
		out[i] = strings.ToLower(p.Email)
	}
	slices.Sort(out)
	return out
}

func addresses(people []*person) []string {
	out := make([]string, len(people))
	for i, p := range people {
		out[i] = p.address
	}
	slices.Sort(out)
	return out
}

// ── Acts ────────────────────────────────────────────────────────────────────

func (w *world) startsChat(ctx context.Context, author, with, body string) error {
	a, err := w.person(author)
	if err != nil {
		return err
	}
	b, err := w.person(with)
	if err != nil {
		return err
	}
	chatID, err := a.session.addChat(ctx, "direct", []string{b.address}, "")
	if err != nil {
		return fmt.Errorf("%s starts a chat with %s: %w", author, with, err)
	}
	w.chats = append(w.chats, &chatRoom{members: []*person{a, b}})
	if _, err := a.session.addChatMessage(ctx, chatID, body); err != nil {
		return fmt.Errorf("%s writes in the chat with %s: %w", author, with, err)
	}
	return nil
}

// startedChat is "starts a chat" as a Given: it waits until the other person
// has the chat and its first message.
func (w *world) startedChat(ctx context.Context, author, with, body string) error {
	if err := w.startsChat(ctx, author, with, body); err != nil {
		return err
	}
	return w.hasChatWithLast(ctx, 30, with, author, body)
}

func (w *world) startedGroupChat(ctx context.Context, author, name, list string) error {
	a, err := w.person(author)
	if err != nil {
		return err
	}
	others, err := w.persons(list)
	if err != nil {
		return err
	}
	if _, err := a.session.addChat(ctx, "group", addresses(others), name); err != nil {
		return fmt.Errorf("%s starts the group chat %q: %w", author, name, err)
	}
	r := &chatRoom{name: name, members: append([]*person{a}, others...)}
	w.chats = append(w.chats, r)
	return eventually(ctx, 30, func() error {
		var errs []error
		for _, p := range r.members {
			_, err := p.chatOf(ctx, r)
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	})
}

func (w *world) writesInChat(ctx context.Context, who, with, name, body string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.room(p, with, name)
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	if _, err := p.session.addChatMessage(ctx, c.ChatID, body); err != nil {
		return fmt.Errorf("%s writes in %s: %w", who, r, err)
	}
	return nil
}

// wroteInChat is "writes in" as a Given: it waits until every member has it.
func (w *world) wroteInChat(ctx context.Context, who, name, body string) error {
	if err := w.writesInChat(ctx, who, "", name, body); err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	return eventually(ctx, 30, func() error {
		var errs []error
		for _, p := range r.members {
			errs = append(errs, p.seesInChat(ctx, r, body))
		}
		return errors.Join(errs...)
	})
}

func (w *world) addsToChat(ctx context.Context, who, added, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	n, err := w.person(added)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	if _, err := p.session.addChatParticipant(ctx, c.ChatID, n.address); err != nil {
		return fmt.Errorf("%s adds %s to %s: %w", who, added, r, err)
	}
	r.members = append(r.members, n)
	return nil
}

// leavesChat removes the person's own participant row, which is what leaving
// is in the API.
func (w *world) leavesChat(ctx context.Context, who, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	participants, err := p.session.chatParticipants(ctx, c.ChatID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(participants, func(x chatParticipant) bool { return strings.EqualFold(x.Email, p.address) })
	if i < 0 {
		return fmt.Errorf("%s is not a participant of %s", who, r)
	}
	if err := p.session.removeChatParticipant(ctx, participants[i].ParticipantID); err != nil {
		return fmt.Errorf("%s leaves %s: %w", who, r, err)
	}
	r.members = slices.DeleteFunc(r.members, func(m *person) bool { return m == p })
	return nil
}

func (w *world) deletesChat(ctx context.Context, who, with string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.room(p, with, "")
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	return p.session.deleteChat(ctx, c.ChatID)
}

// triesToWriteInChat writes where the story expects a refusal. A chat the
// person can no longer see at all counts as refused too.
func (w *world) triesToWriteInChat(ctx context.Context, who, name, body string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		w.lastAttempt.status, w.lastAttempt.code = http.StatusNotFound, ""
		return nil
	}
	w.lastAttempt.status, w.lastAttempt.code, err = p.session.try(ctx, http.MethodPost,
		fmt.Sprintf("/api/v1/chat/message/%d", c.ChatID), map[string]any{"body": body, "attachments": []upload{}})
	return err
}

func (w *world) opensChat(ctx context.Context, list, name string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	for _, p := range people {
		c, err := p.chatOf(ctx, r)
		if err != nil {
			return err
		}
		if err := p.session.markChatRead(ctx, c.ChatID); err != nil {
			return fmt.Errorf("%s opens %s: %w", p.name, r, err)
		}
	}
	return nil
}

// ── Outcomes ────────────────────────────────────────────────────────────────

func (w *world) hasChatWithLast(ctx context.Context, seconds int, who, with, body string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	other, err := w.person(with)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		r, err := w.direct(p, other)
		if err != nil {
			return err
		}
		c, err := p.chatOf(ctx, r)
		if err != nil {
			return err
		}
		if c.LastMessage == nil || c.LastMessage.Body != body {
			return fmt.Errorf("%s's chat with %s ends with %+v, want %q", who, with, c.LastMessage, body)
		}
		return nil
	})
}

func (w *world) unreadInChat(ctx context.Context, who string, want int64, with string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.room(p, with, "")
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	if c.UnreadCount != want {
		return fmt.Errorf("%s has %d unread in %s, want %d", who, c.UnreadCount, r, want)
	}
	return nil
}

// chatReadsTheSame checks every member's copy of a chat holds exactly the
// table's messages, in order, from those senders.
func (w *world) chatReadsTheSame(ctx context.Context, seconds, a, b, name string, table *godog.Table) error {
	var r *chatRoom
	var err error
	if name != "" {
		r, err = w.group(name)
	} else {
		var pa, pb *person
		if pa, err = w.person(a); err != nil {
			return err
		}
		if pb, err = w.person(b); err != nil {
			return err
		}
		r, err = w.direct(pa, pb)
	}
	if err != nil {
		return err
	}
	if table == nil || len(table.Rows) < 2 {
		return fmt.Errorf("the step needs a table of from and message")
	}
	var want []string
	for _, row := range table.Rows[1:] {
		from, err := w.person(strings.TrimSpace(row.Cells[0].Value))
		if err != nil {
			return err
		}
		want = append(want, from.address+": "+strings.TrimSpace(row.Cells[1].Value))
	}
	check := func() error {
		var errs []error
		for _, p := range r.members {
			c, err := p.chatOf(ctx, r)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			messages, err := p.session.chatMessages(ctx, c.ChatID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			var got []string
			for _, m := range messages {
				got = append(got, strings.ToLower(m.SenderEmail)+": "+m.Body)
			}
			if !slices.Equal(got, want) {
				errs = append(errs, fmt.Errorf("%s's %s reads %q, want %q", p.name, r, got, want))
			}
		}
		return errors.Join(errs...)
	}
	if seconds == "" {
		return check()
	}
	n, err := strconv.Atoi(seconds)
	if err != nil {
		return err
	}
	return eventually(ctx, n, check)
}

func (w *world) haveNoChats(ctx context.Context, list string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range people {
		rows, err := p.session.chats(ctx)
		if err != nil {
			errs = append(errs, err)
		} else if len(rows) > 0 {
			errs = append(errs, fmt.Errorf("%s has %d chats, want none", p.name, len(rows)))
		}
	}
	return errors.Join(errs...)
}

func (w *world) haveGroupChat(ctx context.Context, seconds int, list, name, memberList string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	members, err := w.persons(memberList)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	want := addresses(members)
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			c, err := p.chatOf(ctx, r)
			if err != nil {
				errs = append(errs, err)
			} else if got := c.members(); !slices.Equal(got, want) {
				errs = append(errs, fmt.Errorf("%s sees %v in %s, want %v", p.name, got, r, want))
			}
		}
		return errors.Join(errs...)
	})
}

func (w *world) chatMembersAre(ctx context.Context, seconds int, name, list string) error {
	r, err := w.group(name)
	if err != nil {
		return err
	}
	members, err := w.persons(list)
	if err != nil {
		return err
	}
	want := addresses(members)
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range r.members {
			c, err := p.chatOf(ctx, r)
			if err != nil {
				errs = append(errs, err)
			} else if got := c.members(); !slices.Equal(got, want) {
				errs = append(errs, fmt.Errorf("%s sees %v in %s, want %v", p.name, got, r, want))
			}
		}
		return errors.Join(errs...)
	})
}

// seesInChat checks the person's copy of the room holds a message with body.
func (p *person) seesInChat(ctx context.Context, r *chatRoom, body string) error {
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	messages, err := p.session.chatMessages(ctx, c.ChatID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(messages, func(m chatMessage) bool { return m.Body == body }) {
		return fmt.Errorf("%s does not see %q in %s", p.name, body, r)
	}
	return nil
}

func (w *world) eventuallySeeInChat(ctx context.Context, seconds int, list, body, name string) error {
	people, err := w.persons(list)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	return eventually(ctx, seconds, func() error {
		var errs []error
		for _, p := range people {
			errs = append(errs, p.seesInChat(ctx, r, body))
		}
		return errors.Join(errs...)
	})
}

func (w *world) seesInChat(ctx context.Context, who, body, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	return p.seesInChat(ctx, r, body)
}

// neverSeesInChat runs after others have seen the message, so a copy that
// would have arrived has had the time to.
func (w *world) neverSeesInChat(ctx context.Context, who, body, name string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.group(name)
	if err != nil {
		return err
	}
	if p.seesInChat(ctx, r, body) == nil {
		return fmt.Errorf("%s sees %q in %s", who, body, r)
	}
	return nil
}

// nobodySeesInChat checks everyone in the story, members or not.
func (w *world) nobodySeesInChat(ctx context.Context, body, name string) error {
	r, err := w.group(name)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range w.everyone() {
		if p.seesInChat(ctx, r, body) == nil {
			errs = append(errs, fmt.Errorf("%s sees %q in %s", p.name, body, r))
		}
	}
	return errors.Join(errs...)
}

func (w *world) hasNoChatWith(ctx context.Context, who, with string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.room(p, with, "")
	if err != nil {
		return err
	}
	if _, err := p.chatOf(ctx, r); err == nil {
		return fmt.Errorf("%s still has %s", who, r)
	}
	return nil
}

func (w *world) stillHasChatWith(ctx context.Context, who, with, list string) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	r, err := w.room(p, with, "")
	if err != nil {
		return err
	}
	members, err := w.persons(list)
	if err != nil {
		return err
	}
	c, err := p.chatOf(ctx, r)
	if err != nil {
		return err
	}
	if got, want := c.members(), addresses(members); !slices.Equal(got, want) {
		return fmt.Errorf("%s sees %v in %s, want %v", who, got, r, want)
	}
	return nil
}

// seesReceipts checks the ticks on the sender's own message, as the sender's
// Domain reports them: how many of the other members have it, and how many
// have read it. The sender's own copy is left out of every count.
func (w *world) seesReceipts(ctx context.Context, seconds int, who, body string, delivered, read, others int) error {
	p, err := w.person(who)
	if err != nil {
		return err
	}
	var r *chatRoom
	for _, room := range w.chats {
		if slices.Contains(room.members, p) {
			r = room
		}
	}
	if r == nil {
		return fmt.Errorf("%s is in no chat in this story", who)
	}
	return eventually(ctx, seconds, func() error {
		c, err := p.chatOf(ctx, r)
		if err != nil {
			return err
		}
		messages, err := p.session.chatMessages(ctx, c.ChatID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(messages, func(m chatMessage) bool {
			return m.Body == body && strings.EqualFold(m.SenderEmail, p.address)
		})
		if i < 0 {
			return fmt.Errorf("%s has no message of her own saying %q in %s", who, body, r)
		}
		st := messages[i].DeliveryStatus
		if st == nil {
			return fmt.Errorf("%s's message %q has no delivery status", who, body)
		}
		got := [3]int{st.Delivered - 1, st.Read - 1, st.Sent - 1}
		if want := [3]int{delivered, read, others}; got != want {
			return fmt.Errorf("%s's message %q is delivered to %d and read by %d of %d others, want %d and %d of %d",
				who, body, got[0], got[1], got[2], delivered, read, others)
		}
		return nil
	})
}
