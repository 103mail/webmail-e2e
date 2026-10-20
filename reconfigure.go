package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The controls that change how a Domain is configured, and the fault proxy
// every federation request passes through. Like taking a Domain down, they
// act on the test stack and never on a server (ADR 20 in webmail-server).

// configVariable is the compose variable that swaps a Domain's
// federation.json (deploy/docker-compose.yml).
func configVariable(domain string) string {
	return "FEDERATION_CONFIG_" + regexp.MustCompile(`[^A-Z0-9]`).ReplaceAllString(strings.ToUpper(domain), "_")
}

// federationConfig reads the federation.json a Domain runs with by default.
func (s *Stack) federationConfig(domain string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(s.deployDir, "domain", domain, "federation.json"))
	if err != nil {
		return nil, fmt.Errorf("read the federation config of %s: %w", domain, err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse the federation config of %s: %w", domain, err)
	}
	return cfg, nil
}

// reconfigure runs a Domain's API and worker with a changed federation
// config, or with its own again when cfg is nil, and waits until it answers.
func (s *Stack) reconfigure(ctx context.Context, domain string, cfg map[string]any) error {
	override := filepath.Join(s.deployDir, "domain", domain, "federation.e2e.json")
	value := ""
	if cfg != nil {
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(override, raw, 0o644); err != nil {
			return fmt.Errorf("write the federation config of %s: %w", domain, err)
		}
		value = "./domain/" + domain + "/federation.e2e.json"
	}
	services, err := s.services(ctx, domain, "api-", "worker-")
	if err != nil {
		return err
	}
	args := append([]string{"compose", "-f", filepath.Join(s.deployDir, "docker-compose.yml"),
		"up", "-d", "--no-deps", "--force-recreate"}, services...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), configVariable(domain)+"="+value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("recreate %s with a changed config: %w: %s", domain, err, out)
	}
	if cfg == nil {
		os.Remove(override)
	}
	return pollReal(ctx, time.Minute, func() error { return s.health(ctx, domain) })
}

// services lists a Domain's compose services whose names start with one of
// the prefixes.
func (s *Stack) services(ctx context.Context, domain string, prefixes ...string) ([]string, error) {
	ids, err := s.containers(ctx, domain)
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, "docker", append([]string{"inspect", "--format",
		`{{index .Config.Labels "com.docker.compose.service"}}`}, ids...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("find the services of %s: %w", domain, err)
	}
	var services []string
	for _, service := range strings.Fields(string(out)) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(service, prefix) {
				services = append(services, service)
			}
		}
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("%s has no %v services", domain, prefixes)
	}
	return services, nil
}

// rotateKey gives a Domain a new signing key and publishes its current one as
// a previous key, the way an operator rotates (RFC 9.5). The new key file is
// returned so it can be removed after the story.
func (s *Stack) rotateKey(ctx context.Context, domain string, cfg map[string]any) (string, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.ed25519.%d.pem", domain, time.Now().UnixNano())
	path := filepath.Join(s.deployDir, "keys", name)
	// Readable by the servers, which run as another user.
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o644); err != nil {
		return "", fmt.Errorf("write a new key for %s: %w", domain, err)
	}
	domains, _ := cfg["domains"].([]any)
	if len(domains) != 1 {
		return path, fmt.Errorf("the federation config of %s has %d domains, want 1", domain, len(domains))
	}
	d := domains[0].(map[string]any)
	previous, _ := d["previousKeyFiles"].([]any)
	d["previousKeyFiles"] = append([]any{d["privateKeyFile"]}, previous...)
	d["privateKeyFile"] = "/etc/webmail/keys/" + name
	return path, s.reconfigure(ctx, domain, cfg)
}

// fault is one rule for the fault proxy (deploy/faults in webmail-server).
type fault struct {
	Domain     string `json:"domain"`
	From       string `json:"from,omitempty"`
	Action     string `json:"action"`
	Status     int    `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	RetryAfter int    `json:"retryAfter,omitempty"`
	Count      int    `json:"count"`
}

// faultsURL is where the fault proxy takes its rules.
func faultsURL() string {
	if url := os.Getenv("WEBMAIL_E2E_FAULTS"); url != "" {
		return url
	}
	return "http://127.0.0.1:8026"
}

// arm adds a rule to the fault proxy.
func (s *Stack) arm(ctx context.Context, f fault) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, faultsURL()+"/faults", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("the fault proxy is not running: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the fault proxy refused %s: %d", raw, resp.StatusCode)
	}
	return nil
}

// clearFaults removes every rule for a Domain, or every rule at all when
// domain is empty.
func (s *Stack) clearFaults(ctx context.Context, domain string) error {
	url := faultsURL() + "/faults"
	if domain != "" {
		url += "?domain=" + domain
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("the fault proxy is not running: %w", err)
	}
	resp.Body.Close()
	return nil
}
