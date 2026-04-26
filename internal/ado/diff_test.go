package ado

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetFileDiffBothSides(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/_apis/git/repositories/repo-uuid/items") {
			t.Fatalf("path: %s", r.URL.Path)
		}
		v := r.URL.Query().Get("versionDescriptor.version")
		var body string
		switch v {
		case "src-sha":
			body = "new line\nshared\n"
		case "tgt-sha":
			body = "old line\nshared\n"
		default:
			t.Fatalf("unexpected version: %s", v)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content":         base64.StdEncoding.EncodeToString([]byte(body)),
			"contentMetadata": map[string]any{"encoding": "base64"},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	src, tgt, err := c.GetFileContents(context.Background(), "repo-uuid", "/src/login.go", "src-sha", "tgt-sha")
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != "new line\nshared\n" || string(tgt) != "old line\nshared\n" {
		t.Fatalf("src=%q tgt=%q", src, tgt)
	}
}
