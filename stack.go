// Package e2e drives the federated webmail stack through its public API. The
// stories in features/ are the specification; this package is the small set of
// step definitions that turns their phrases into API calls.
package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// password is what every account the suite creates signs in with.
const password = "password"

// passwordHash is bcrypt of password, the same hash deploy/seed.sh writes, so
// the suite needs no bcrypt of its own.
const passwordHash = "$2y$10$jO3R170QUVr5pj.eMO1bZuduB8SoSAuIpC1jrmtzwnh6vEWr.bqpW"

// Stack is the running compose stack: one Server per Domain, each reached at
// https://webmail.{domain} and trusted through the stack's local CA.
type Stack struct {
	deployDir string
	http      *http.Client

	mu      sync.Mutex
	healthy map[string]error
}

// NewStack returns a Stack for the compose files in deployDir.
func NewStack(deployDir string) (*Stack, error) {
	caFile := filepath.Join(deployDir, "certs", "webmail-local-ca.crt")
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read the stack's CA (run deploy/setup.sh first): %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificate in %s", caFile)
	}
	return &Stack{
		deployDir: deployDir,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		},
		healthy: map[string]error{},
	}, nil
}

// baseURL is the Federation hostname of a Domain (RFC 1.3).
func (s *Stack) baseURL(domain string) string {
	return "https://webmail." + domain
}

// Check reports whether a Domain is up. The answer is cached for the run, so a
// Domain missing from the stack fails every story that needs it with the same
// message rather than a timeout each.
func (s *Stack) Check(ctx context.Context, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.healthy[domain]; ok {
		return err
	}
	err := s.health(ctx, domain)
	if err != nil {
		err = fmt.Errorf("Domain %s is not running in the test stack: %w", domain, err)
	}
	s.healthy[domain] = err
	return err
}

func (s *Stack) health(ctx context.Context, domain string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL(domain)+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /health answered %d", resp.StatusCode)
	}
	return nil
}

// Provision creates an account on a Domain with one mailbox, an Inbox and an
// alias for address, which is also the login.
//
// There is no signup endpoint yet, so this writes the rows into the Domain's
// SQLite file directly, the same rows deploy/seed.sh writes. It is a stack
// control like taking a Domain down, not an observation: no step reads the
// database to decide an outcome. Replace it with the signup API when M4 lands.
func (s *Stack) Provision(ctx context.Context, domain, address, name string) error {
	path := filepath.Join(s.deployDir, "data", domain, "webmail.db")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no database for Domain %s: %w", domain, err)
	}
	db, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	defer tx.Rollback()

	insert := func(query string, args ...any) (int64, error) {
		var id int64
		err := tx.QueryRowContext(ctx, query+` RETURNING "id"`, args...).Scan(&id)
		return id, err
	}
	customerID, err := insert(`INSERT INTO "customer" ("name") VALUES (?)`, name)
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	userID, err := insert(`INSERT INTO "user" ("customer_id", "username", "password", "name") VALUES (?,?,?,?)`,
		customerID, address, passwordHash, name)
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	mailboxID, err := insert(`INSERT INTO "mailbox" ("customer_id", "user_id", "name", "color") VALUES (?,?,?,?)`,
		customerID, userID, address, "red")
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	folderID, err := insert(`INSERT INTO "folder" ("customer_id", "mailbox_id", "name") VALUES (?,?,?)`,
		customerID, mailboxID, "Inbox")
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	aliasID, err := insert(`INSERT INTO "alias" ("customer_id", "email", "name", "mailbox_id") VALUES (?,?,?,?)`,
		customerID, address, name, mailboxID)
	if err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE "mailbox" SET "default_folder_id" = ?, "default_alias_id" = ? WHERE "id" = ?`,
		folderID, aliasID, mailboxID); err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE "customer" SET "default_mailbox_id" = ? WHERE "id" = ?`,
		mailboxID, customerID); err != nil {
		return fmt.Errorf("provision %s: %w", address, err)
	}
	return tx.Commit()
}
