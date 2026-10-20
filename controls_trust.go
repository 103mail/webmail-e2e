package e2e

import (
	"context"
	"net/url"
)

// More of the read-only controls (ADR 20 in webmail-server): which peer keys
// a Domain pinned, how often it stores a file's bytes, and who is on its
// federated threads.

// pinnedPeer is a peer descriptor a Domain trusts.
type pinnedPeer struct {
	Domain             string   `json:"domain"`
	DescriptorURL      string   `json:"descriptorUrl"`
	PublicKey          string   `json:"publicKey"`
	PreviousPublicKeys []string `json:"previousPublicKeys"`
}

// storedFile is how a Domain holds one file's bytes.
type storedFile struct {
	HashMD5     string `json:"hashMd5"`
	FileSize    int64  `json:"fileSize"`
	Attachments int64  `json:"attachments"`
	Stored      bool   `json:"stored"`
}

// federatedThread is a thread's federation id and members on one Domain.
type federatedThread struct {
	FederationID string   `json:"federationId"`
	Members      []string `json:"members"`
}

func (s *Stack) peers(ctx context.Context, domain string) (map[string]pinnedPeer, error) {
	var list []pinnedPeer
	if err := s.control(ctx, domain, "/control/peers", &list); err != nil {
		return nil, err
	}
	out := map[string]pinnedPeer{}
	for _, p := range list {
		out[p.Domain] = p
	}
	return out, nil
}

func (s *Stack) storedFiles(ctx context.Context, domain, md5 string) ([]storedFile, error) {
	var out []storedFile
	err := s.control(ctx, domain, "/control/files?md5="+url.QueryEscape(md5), &out)
	return out, err
}

func (s *Stack) threadsOf(ctx context.Context, domain, member string) ([]federatedThread, error) {
	var out []federatedThread
	err := s.control(ctx, domain, "/control/threads?member="+url.QueryEscape(member), &out)
	return out, err
}
