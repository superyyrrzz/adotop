// Package cache persists the most recent ADO responses to ~/.adotop/cache
// so the UI can paint instantly on startup while a fresh fetch runs in
// the background.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/renzeyu/adotop/internal/ado"
)

const schemaVersion = 1

// Dir returns ~/.adotop/cache.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".adotop", "cache"), nil
}

type Identity struct {
	Schema      int    `json:"schema"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
}

type ListSnapshot struct {
	Schema int              `json:"schema"`
	PRs    []ado.PRSummary `json:"prs"`
}

// Store writes/reads cache files. It owns no state beyond the base dir,
// so callers can construct a new one cheaply.
type Store struct {
	base string
}

func New() (*Store, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("mkdir %s: %w", d, err)
	}
	return &Store{base: d}, nil
}

func (s *Store) identityPath() string { return filepath.Join(s.base, "identity.json") }

func (s *Store) listPath(org, project string, tab ado.Tab) string {
	name := fmt.Sprintf("list-%s-%s-%d.json", safe(org), safe(project), int(tab))
	return filepath.Join(s.base, name)
}

func safe(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(s)
}

func (s *Store) LoadIdentity() (Identity, bool) {
	var id Identity
	if !readJSON(s.identityPath(), &id) || id.Schema != schemaVersion || id.UserID == "" {
		return Identity{}, false
	}
	return id, true
}

func (s *Store) SaveIdentity(userID, displayName string) error {
	return writeJSON(s.identityPath(), Identity{Schema: schemaVersion, UserID: userID, DisplayName: displayName})
}

func (s *Store) LoadList(org, project string, tab ado.Tab) ([]ado.PRSummary, bool) {
	var snap ListSnapshot
	if !readJSON(s.listPath(org, project, tab), &snap) || snap.Schema != schemaVersion {
		return nil, false
	}
	return snap.PRs, true
}

func (s *Store) SaveList(org, project string, tab ado.Tab, prs []ado.PRSummary) error {
	return writeJSON(s.listPath(org, project, tab), ListSnapshot{Schema: schemaVersion, PRs: prs})
}

func readJSON(path string, out any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, out) == nil
}

func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
