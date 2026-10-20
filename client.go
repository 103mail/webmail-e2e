package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Session is one person signed in to their own Domain's public API.
type Session struct {
	stack      *Stack
	domain     string
	token      string
	customerID int64
}

// The shapes below mirror webmail-server's internal/email JSON, keeping only
// the fields the steps look at.

type party struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type mailbox struct {
	MailboxID int64 `json:"mailboxId"`
	IsDefault bool  `json:"isDefault"`
}

type folder struct {
	FolderID  int64  `json:"folderId"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type thread struct {
	ThreadID    int64   `json:"threadId"`
	Subject     string  `json:"subject"`
	UnreadCount int64   `json:"unreadCount"`
	Subscribers []party `json:"subscribers"`
}

type message struct {
	MessageID   int64           `json:"messageId"`
	SenderEmail string          `json:"senderEmail"`
	Body        string          `json:"body"`
	ReadAt      *string         `json:"readAt"`
	Attachments []attachmentRef `json:"attachments"`
}

// attachmentRef is a file on a message, as the message list shows it.
type attachmentRef struct {
	AttachmentID int64  `json:"attachmentId"`
	FileName     string `json:"fileName"`
	FileSize     int64  `json:"fileSize"`
}

// upload is a file to attach, as the compose endpoints take it. The bytes go
// as base64, which is what []byte becomes in JSON.
type upload struct {
	FileName string `json:"fileName"`
	MimeType string `json:"mimeType"`
	FileData []byte `json:"fileData"`
}

type contact struct {
	Email string `json:"email"`
}

// Login signs in and learns the account's customer id, which several
// endpoints take in the path.
func (s *Stack) Login(ctx context.Context, domain, address string) (*Session, error) {
	sess := &Session{stack: s, domain: domain}
	var tok struct {
		Value string `json:"value"`
	}
	if err := sess.do(ctx, http.MethodPost, "/api/v1/auth/token",
		map[string]string{"email": address, "password": password}, &tok); err != nil {
		return nil, fmt.Errorf("sign in as %s: %w", address, err)
	}
	sess.token = tok.Value
	var user struct {
		CustomerID int64 `json:"customerId"`
	}
	if err := sess.do(ctx, http.MethodGet, "/api/v1/auth/token", nil, &user); err != nil {
		return nil, fmt.Errorf("look up %s: %w", address, err)
	}
	sess.customerID = user.CustomerID
	return sess, nil
}

func (s *Session) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.stack.baseURL(s.domain)+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.stack.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		refusal := &apiError{status: resp.StatusCode,
			message: fmt.Sprintf("%s %s answered %d: %s", method, path, resp.StatusCode, bytes.TrimSpace(raw))}
		var answer struct {
			Error  string `json:"error"`
			Detail string `json:"detail"`
		}
		if json.Unmarshal(raw, &answer) == nil {
			refusal.code, refusal.detail = answer.Error, answer.Detail
		}
		return refusal
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: decode %q: %w", method, path, raw, err)
	}
	return nil
}

// apiError is an answer other than 200, with the error code and detail the
// API gave, so a step that expects a refusal can read why.
type apiError struct {
	status       int
	code, detail string
	message      string
}

func (e *apiError) Error() string { return e.message }

// created checks the id a POST answers with. The API reports a create it
// refused as 0 with status 200, so 0 is a failure here.
func created(what string, id int64, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, fmt.Errorf("%s was refused (the API answered 0)", what)
	}
	return id, nil
}

func (s *Session) mailboxes(ctx context.Context) ([]mailbox, error) {
	var out []mailbox
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/mailbox/%d", s.customerID), nil, &out)
	return out, err
}

func (s *Session) folders(ctx context.Context, mailboxID int64) ([]folder, error) {
	var out []folder
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/folder/%d", mailboxID), nil, &out)
	return out, err
}

func (s *Session) threads(ctx context.Context, folderID int64) ([]thread, error) {
	var out []thread
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/thread/%d", folderID), nil, &out)
	return out, err
}

// messages lists a thread oldest first. The API answers newest first.
// Listing does not mark anything read.
func (s *Session) messages(ctx context.Context, threadID int64) ([]message, error) {
	var out []message
	if err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/message/%d", threadID), nil, &out); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Session) contacts(ctx context.Context) ([]contact, error) {
	var out []contact
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/contact/%d", s.customerID), nil, &out)
	return out, err
}

func (s *Session) addThread(ctx context.Context, folderID int64, subscribers []string, subject, body string,
	files ...upload) (int64, error) {
	var id int64
	err := s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/email/thread/%d", folderID),
		map[string]any{"subject": subject, "subscribers": subscribers, "body": body, "attachments": uploads(files)}, &id)
	return created("new thread", id, err)
}

func (s *Session) addMessage(ctx context.Context, threadID int64, body string, files ...upload) (int64, error) {
	var id int64
	err := s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/email/message/%d", threadID),
		map[string]any{"body": body, "attachments": uploads(files)}, &id)
	return created("reply", id, err)
}

// uploads sends an empty list rather than null when there are no files.
func uploads(files []upload) []upload {
	if files == nil {
		return []upload{}
	}
	return files
}

// download reads an attachment's bytes as the person sees them.
func (s *Session) download(ctx context.Context, attachmentID int64) ([]byte, error) {
	var out struct {
		FileData []byte `json:"fileData"`
	}
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/email/attachment/%d", attachmentID), nil, &out)
	return out.FileData, err
}

func (s *Session) addSubscriber(ctx context.Context, threadID int64, address string) (int64, error) {
	var id int64
	err := s.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/email/subscriber/%d", threadID),
		map[string]any{"email": address}, &id)
	return created("invitation", id, err)
}

func (s *Session) markRead(ctx context.Context, messageID int64) error {
	var ok bool
	if err := s.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/email/message/%d", messageID), map[string]any{}, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("marking message %d read was refused", messageID)
	}
	return nil
}

// try makes a call that the story expects may be refused, and returns the
// status and the error code of the answer instead of failing on them.
func (s *Session) try(ctx context.Context, method, path string, in any) (int, string, error) {
	status, code, _, err := s.tryDetail(ctx, method, path, in)
	return status, code, err
}

// tryDetail is try, and also returns the detail the answer gave.
func (s *Session) tryDetail(ctx context.Context, method, path string, in any) (int, string, string, error) {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return 0, "", "", err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.stack.baseURL(s.domain)+path, body)
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.stack.http.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	var answer struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &answer)
	return resp.StatusCode, answer.Error, answer.Detail, nil
}

func (s *Session) leave(ctx context.Context, threadID int64, address string) error {
	var ok bool
	return s.do(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v1/email/subscriber/%d?email=%s", threadID, url.QueryEscape(address)), nil, &ok)
}
