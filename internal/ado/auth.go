package ado

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// TokenProvider returns a valid bearer token, fetching/refreshing as needed.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

// Sentinel auth errors. The CLI's pre-flight check and the TUI's
// footer-error renderer both look for these via errors.Is so they
// can show actionable, friendly messages instead of the raw "exec:
// az: not found" / "Please run 'az login'" leakage.
var (
	// ErrAzNotInstalled fires when `az` isn't on PATH at all.
	ErrAzNotInstalled = errors.New("az CLI not installed")
	// ErrAzNotLoggedIn fires when `az` runs but reports the user is
	// not authenticated (typically "Please run 'az login' to setup
	// account" in stderr).
	ErrAzNotLoggedIn = errors.New("az CLI not logged in")
)

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
	ExpiresOn   string `json:"expiresOn"`  // legacy: "2026-04-25 15:00:00.000000"
	Expires_On  int64  `json:"expires_on"` // newer az versions: unix seconds
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
		// Classify the error so callers can surface a friendly message.
		// exec.ErrNotFound covers "az not on PATH"; the stderr from a
		// running-but-failing az covers "not logged in" and other auth
		// states. Wrapping with %w preserves the original for logs.
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %v", ErrAzNotInstalled, err)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := string(ee.Stderr)
			if isAzLoginNeeded(stderr) {
				return "", fmt.Errorf("%w: %s", ErrAzNotLoggedIn, strings.TrimSpace(stderr))
			}
			return "", fmt.Errorf("az get-access-token: %w: %s", err, stderr)
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

// isAzLoginNeeded recognizes the stderr az prints when the user has
// no cached credentials. Match is loose because az's exact wording
// has shifted across versions ("Please run 'az login'", "az login",
// "AADSTS50058 ... no logged-in user", etc.). Whichever variant we
// see, the actionable answer is the same: run az login.
func isAzLoginNeeded(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "az login") ||
		strings.Contains(s, "no subscriptions") ||
		strings.Contains(s, "please run") ||
		strings.Contains(s, "aadsts50058") ||
		strings.Contains(s, "credentials are required")
}
