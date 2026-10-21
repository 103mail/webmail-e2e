package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A sender speaks the federation protocol as one Domain, with that Domain's
// own key from the stack's deploy/keys. It is how a story acts as the test
// peer, peer.test, or as a real Domain trying to speak for another. It signs
// exactly the way webmail-server does (RFC 6.2), so whatever a Domain
// refuses, it refuses for the reason the story names and not for a mistake
// in the harness.

// protocolVersion is the only version the stack speaks.
const protocolVersion = "0.2"

type sender struct {
	stack  *Stack
	domain string
	key    ed25519.PrivateKey
}

// sender loads a Domain's signing key from the stack.
func (s *Stack) sender(domain string) (*sender, error) {
	path := filepath.Join(s.deployDir, "keys", domain+".ed25519.pem")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the key of %s (run deploy/setup.sh): %w", domain, err)
	}
	key, err := parseKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &sender{stack: s, domain: domain, key: key}, nil
}

func parseKey(raw []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("holds a %T, want an Ed25519 key", parsed)
	}
	return key, nil
}

// wireEnvelope is an envelope as RFC 5.2 lays it out.
type wireEnvelope struct {
	Version      string           `json:"version"`
	EnvelopeID   string           `json:"envelope_id"`
	OriginDomain string           `json:"origin_domain"`
	OriginSentAt string           `json:"origin_sent_at"`
	Thread       wireThread       `json:"thread"`
	Message      wireMessage      `json:"message"`
	Attachments  []wireAttachment `json:"attachments"`
	Recipients   []string         `json:"recipients"`
}

type wireThread struct {
	FederationID string   `json:"federation_id"`
	Subject      string   `json:"subject"`
	InReplyTo    *string  `json:"in_reply_to"`
	Members      []string `json:"members,omitempty"`
}

type wireMessage struct {
	FederationID string `json:"federation_id"`
	SenderEmail  string `json:"sender_email"`
	SenderName   string `json:"sender_name,omitempty"`
	Body         string `json:"body"`
	SentAt       string `json:"sent_at"`
}

type wireAttachment struct {
	FederationID string `json:"federation_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	HashSHA256   string `json:"hash_sha256_hex"`
	Data         string `json:"data"`
}

// envelope is a well-formed envelope from this sender's Domain: a message
// from senderEmail to recipients on a thread, new unless threadID is given.
func (p *sender) envelope(senderEmail, threadID, subject, body string, recipients []string) *wireEnvelope {
	if threadID == "" {
		threadID = newUUID()
	}
	now := timestamp(time.Now())
	members := append([]string{senderEmail}, recipients...)
	return &wireEnvelope{
		Version: protocolVersion, EnvelopeID: newUUID(), OriginDomain: p.domain, OriginSentAt: now,
		Thread:      wireThread{FederationID: threadID, Subject: subject, Members: members},
		Message:     wireMessage{FederationID: newUUID(), SenderEmail: senderEmail, Body: body, SentAt: now},
		Attachments: []wireAttachment{},
		Recipients:  recipients,
	}
}

// attach adds a file to an envelope with its true size and hash.
func (e *wireEnvelope) attach(name, mimeType string, data []byte) {
	sum := sha256.Sum256(data)
	e.Attachments = append(e.Attachments, wireAttachment{
		FederationID: newUUID(), FileName: name, MimeType: mimeType, FileSize: int64(len(data)),
		HashSHA256: hex.EncodeToString(sum[:]), Data: base64.StdEncoding.EncodeToString(data),
	})
}

// request is one inbound POST, signed or deliberately not.
type request struct {
	body            []byte
	version         string
	nonce           string
	date            time.Time
	contentType     string
	contentEncoding string
	// key signs the request; nil signs with the sender's own key.
	key ed25519.PrivateKey
	// tamper changes the body after it was signed.
	tamper func([]byte) []byte
}

// request starts a correct request carrying an envelope.
func (p *sender) request(e *wireEnvelope) (*request, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	return &request{body: body, version: protocolVersion, nonce: nonce, date: time.Now(),
		contentType: "application/json; charset=utf-8"}, nil
}

// answer is what a Domain said to a federation request.
type answer struct {
	status int
	code   string
	detail string
	// retryAfter is the Retry-After header, which a 429 carries.
	retryAfter string
}

func (a answer) String() string {
	return fmt.Sprintf("%d %q (%s)", a.status, a.code, a.detail)
}

// send POSTs a request to a Domain's inbound endpoint and reads the answer.
func (p *sender) send(ctx context.Context, destination string, r *request) (answer, error) {
	key := r.key
	if key == nil {
		key = p.key
	}
	date := r.date.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	input := []byte(strings.Join([]string{r.version, p.domain, r.nonce, date}, "\n") + "\n")
	signature := "ed25519:" + base64.StdEncoding.EncodeToString(ed25519.Sign(key, append(input, r.body...)))
	body := r.body
	if r.tamper != nil {
		body = r.tamper(bytes.Clone(body))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.stack.baseURL(destination)+"/api/v1/federation/inbound", bytes.NewReader(body))
	if err != nil {
		return answer{}, err
	}
	req.Header.Set("Content-Type", r.contentType)
	if r.contentEncoding != "" {
		req.Header.Set("Content-Encoding", r.contentEncoding)
	}
	req.Header.Set("Date", date)
	req.Header.Set("X-Federation-Version", r.version)
	req.Header.Set("X-Federation-Sender", p.domain)
	req.Header.Set("X-Federation-Nonce", r.nonce)
	req.Header.Set("X-Federation-Signature", signature)
	resp, err := p.stack.http.Do(req)
	if err != nil {
		return answer{}, fmt.Errorf("%s sending to %s: %w", p.domain, destination, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var a struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(raw, &a)
	return answer{status: resp.StatusCode, code: a.Error, detail: a.Detail,
		retryAfter: resp.Header.Get("Retry-After")}, nil
}

// descriptor reads a Domain's published peer descriptor, as a peer would.
func (s *Stack) descriptor(ctx context.Context, domain string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.baseURL(domain)+"/.well-known/webmail-federation", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", domain, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover %s: answered %d", domain, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("discover %s: %w", domain, err)
	}
	return out, nil
}

// newUUID is a version 7 UUID, which is what the RFC asks every id to be.
func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	ms := uint64(time.Now().UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	b[6] = b[6]&0x0f | 0x70
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
