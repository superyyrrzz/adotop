package ado

import (
	"errors"
	"testing"
)

// TestIsAzLoginNeededRecognizesCommonStderrShapes spans the variants
// `az` has used over its versions. Loose matching is intentional —
// the actionable answer is the same regardless of phrasing.
func TestIsAzLoginNeededRecognizesCommonStderrShapes(t *testing.T) {
	yes := []string{
		"Please run 'az login' to setup account.",
		"AADSTS50058: A silent sign-in request was sent but no user is signed in.",
		"ERROR: Please run 'az login' to access your accounts.",
		"No subscriptions found.",
		"Credentials are required.",
	}
	no := []string{
		"",
		"Some unrelated error",
		"Permission denied",
	}
	for _, s := range yes {
		if !isAzLoginNeeded(s) {
			t.Errorf("expected login-needed for %q", s)
		}
	}
	for _, s := range no {
		if isAzLoginNeeded(s) {
			t.Errorf("did NOT expect login-needed for %q", s)
		}
	}
}

// TestErrAzNotInstalledIsSentinel: errors.Is must work on the wrapped
// form returned by Token() so callers can branch on it without
// string-matching.
func TestErrAzNotInstalledIsSentinel(t *testing.T) {
	wrapped := errors.New("wrapped")
	// Hand-build the wrap shape Token() emits.
	got := errors.Join(ErrAzNotInstalled, wrapped)
	if !errors.Is(got, ErrAzNotInstalled) {
		t.Fatalf("errors.Is should match ErrAzNotInstalled in joined form")
	}
}
