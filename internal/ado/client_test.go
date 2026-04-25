package ado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fakeTokens struct {
	calls       atomic.Int32
	invalidated atomic.Int32
}

func (f *fakeTokens) Token(ctx context.Context) (string, error) {
	f.calls.Add(1)
	return "tok", nil
}
func (f *fakeTokens) Invalidate() { f.invalidated.Add(1) }

func TestGetJSONAddsAPIVersion(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	var out map[string]string
	if err := c.GetJSON(context.Background(), "/_apis/connectionData", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out["hello"] != "world" {
		t.Fatalf("body: %v", out)
	}
	if want := "/_apis/connectionData?api-version=" + APIVersion; gotURL != want {
		t.Fatalf("url = %q, want %q", gotURL, want)
	}
}

func TestRetriesOn401Refreshes(t *testing.T) {
	tokens := &fakeTokens{}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()
	c := NewClient("ignored", tokens)
	c.BaseURL = srv.URL
	var out map[string]string
	if err := c.GetJSON(context.Background(), "/x", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if tokens.invalidated.Load() != 1 {
		t.Fatalf("invalidated calls = %d, want 1", tokens.invalidated.Load())
	}
}

func TestAPIErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"bad"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	c.MaxRetries = 0
	err := c.GetJSON(context.Background(), "/x", nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err type = %T, want *APIError; err=%v", err, err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("status = %d", apiErr.Status)
	}
}
