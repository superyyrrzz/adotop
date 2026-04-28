package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// detailCacheCap bounds how many per-PR detail snapshots we keep on
// disk. Each file is ~10–80 KB depending on diff thread count, so 20
// is well under 2 MB. Eviction is LRU by mtime (touched on each
// SaveDetail / read).
const detailCacheCap = 20

// DetailSnapshot is one PR's full detail-screen payload: PR metadata,
// file changes, CI statuses, and discussion threads. Each field can be
// nil if the corresponding fetch hasn't completed yet — the loader
// dispatches them as separate *LoadedMsg events so the renderer treats
// "missing" the same way it does on a cache miss.
type DetailSnapshot struct {
	Schema   int               `json:"schema"`
	PRID     int               `json:"pr_id"`
	Detail   *ado.PRDetail     `json:"detail,omitempty"`
	Files    []ado.FileChange  `json:"files,omitempty"`
	Statuses []ado.StatusCheck `json:"statuses,omitempty"`
	Threads  []ado.Thread      `json:"threads,omitempty"`
}

func (s *Store) detailPath(prID int) string {
	return filepath.Join(s.base, fmt.Sprintf("pr-%d.json", prID))
}

// LoadDetail returns the cached snapshot for prID, or (nil, false) if
// none exists or the schema doesn't match. A read also touches the file
// mtime so the LRU eviction in SaveDetail keeps recently-viewed PRs.
func (s *Store) LoadDetail(prID int) (*DetailSnapshot, bool) {
	if s == nil || prID == 0 {
		return nil, false
	}
	path := s.detailPath(prID)
	var snap DetailSnapshot
	if !readJSON(path, &snap) || snap.Schema != schemaVersion || snap.PRID != prID {
		return nil, false
	}
	// Touch mtime so this PR moves to the front of the LRU. Best-effort:
	// if the chtimes call fails (read-only FS, perms), eviction may
	// drop a PR that was actually just read — annoying but not broken.
	now := nowFn()
	_ = os.Chtimes(path, now, now)
	return &snap, true
}

// SaveDetail persists snap and evicts the oldest PR(s) past detailCacheCap.
// Eviction by file mtime: oldest goes first. Idempotent — repeated saves
// for the same prID just overwrite. Pass a snapshot with PRID==0 and the
// call is a no-op so callers don't have to pre-check.
func (s *Store) SaveDetail(snap DetailSnapshot) error {
	if s == nil || snap.PRID == 0 {
		return nil
	}
	snap.Schema = schemaVersion
	if err := writeJSON(s.detailPath(snap.PRID), snap); err != nil {
		return err
	}
	return s.evictDetailLRU()
}

// DropDetail removes a single PR's cached snapshot. Used when a PR
// transitions to a terminal state (completed/abandoned) — those PRs
// won't change again, so the disk slot is better spent on active ones.
// Missing files are not an error.
func (s *Store) DropDetail(prID int) error {
	if s == nil || prID == 0 {
		return nil
	}
	err := os.Remove(s.detailPath(prID))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// evictDetailLRU walks pr-*.json files and removes the oldest until the
// count is at or under detailCacheCap. Called from SaveDetail; cheap
// because we only ever have a few dozen files at most.
func (s *Store) evictDetailLRU() error {
	entries, err := os.ReadDir(s.base)
	if err != nil {
		return err
	}
	type prFile struct {
		path  string
		mtime int64
	}
	var files []prFile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "pr-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		// Skip non-numeric stems so we don't accidentally remove unrelated
		// files. pr-1234.json -> 1234.
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "pr-"), ".json")
		if _, err := strconv.Atoi(stem); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, prFile{
			path:  filepath.Join(s.base, name),
			mtime: info.ModTime().UnixNano(),
		})
	}
	if len(files) <= detailCacheCap {
		return nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime < files[j].mtime })
	for i := 0; i < len(files)-detailCacheCap; i++ {
		if err := os.Remove(files[i].path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
