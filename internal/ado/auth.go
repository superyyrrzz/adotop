package ado

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// TokenProvider returns a valid bearer token, fetching/refreshing as needed.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

// AzCLITokenProvider shells out to `az account get-access-token`.
type AzCLITokenProvider struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	// skew is how early we treat a token as expired, to avoid races.
	skew time.Duration
}

func NewAzCLITokenProvider() *AzCLITokenProvider {
	return &AzCLITokenProvider{skew: 60 * time.Second}
}

type azTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresOn   string `json:"expiresOn"`   // legacy: "2026-04-25 15:00:00.000000"
	Expires_On  int64  `json:"expires_on"`  // newer az versions: unix seconds
}

func (p *AzCLITokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Until(p.expiresAt) > p.skew {
		return p.token, nil
	}
	cmd := exec.CommandContext(ctx, "az", "account", "get-access-token", "--resource", adoResource, "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("az get-access-token: %w: %s", err, string(ee.Stderr))
		}
		return "", fmt.Errorf("az get-access-token: %w", err)
	}
	var r azTokenResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return "", fmt.Errorf("parse az output: %w", err)
	}
	if r.AccessToken == "" {
		return "", errors.New("az returned empty access token")
	}
	p.token = r.AccessToken
	switch {
	case r.Expires_On > 0:
		p.expiresAt = time.Unix(r.Expires_On, 0)
	case r.ExpiresOn != "":
		// Format used by older az versions; local time, no tz info.
		t, err := time.ParseInLocation("2006-01-02 15:04:05.000000", r.ExpiresOn, time.Local)
		if err == nil {
			p.expiresAt = t
		} else {
			p.expiresAt = time.Now().Add(50 * time.Minute)
		}
	default:
		p.expiresAt = time.Now().Add(50 * time.Minute)
	}
	return p.token, nil
}

func (p *AzCLITokenProvider) Invalidate() {
	p.mu.Lock()
	p.token = ""
	p.expiresAt = time.Time{}
	p.mu.Unlock()
}
