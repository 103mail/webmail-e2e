package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// email is one notification a Domain published, as the mail sink captured it.
// It mirrors mailer.OutboundNotification in webmail-server.
type email struct {
	ID           int    `json:"id"`
	Topic        string `json:"topic"`
	Notification struct {
		From    party   `json:"from"`
		To      []party `json:"to"`
		Bcc     []party `json:"bcc"`
		Subject string  `json:"subject"`
		Text    string  `json:"text"`
		HTML    string  `json:"html"`
		// Raw is the whole RFC 5322 message, base64 in the JSON.
		Raw []byte `json:"raw"`
	} `json:"notification"`
}

// emailsAfter lists what every Domain has sent since the email with id after.
func (s *Stack) emailsAfter(ctx context.Context, after int) ([]email, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/messages?after=%d", s.sinkURL, after), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the mail sink is not running: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the mail sink answered %d", resp.StatusCode)
	}
	var out []email
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("read the mail sink: %w", err)
	}
	return out, nil
}

// lastEmail is the id of the newest email in the sink, or 0 when it is empty.
func (s *Stack) lastEmail(ctx context.Context) (int, error) {
	all, err := s.emailsAfter(ctx, 0)
	if err != nil || len(all) == 0 {
		return 0, err
	}
	return all[len(all)-1].ID, nil
}
