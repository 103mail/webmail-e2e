package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// The read-only controls each Domain's API serves on port 8081 of its own
// address (ADR 20 in webmail-server). They show what crossed a Domain
// boundary, which the public API cannot.

// envelope is one row of a Domain's federation outbox.
type envelope struct {
	ID         int64    `json:"id"`
	PeerDomain string   `json:"peerDomain"`
	Recipients []string `json:"recipients"`
	Status     string   `json:"status"`
	Attempts   int      `json:"attempts"`
}

// account is a user on a Domain and every address that reaches it.
type account struct {
	Username  string   `json:"username"`
	Addresses []string `json:"addresses"`
}

func (s *Stack) control(ctx context.Context, domain, path string, out any) error {
	url := "http://webmail." + domain + ":8081" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("the controls of %s are not reachable: %w", domain, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s answered %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("read %s: %w", url, err)
	}
	return nil
}

func (s *Stack) envelopes(ctx context.Context, domain string) ([]envelope, error) {
	var out []envelope
	err := s.control(ctx, domain, "/control/envelopes", &out)
	return out, err
}

func (s *Stack) accounts(ctx context.Context, domain string) ([]account, error) {
	var out []account
	err := s.control(ctx, domain, "/control/accounts", &out)
	return out, err
}
