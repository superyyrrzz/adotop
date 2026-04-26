// Package gitlocal discovers local clones of ADO repositories and uses them
// to render diffs faster than the REST API can.
package gitlocal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Finder struct {
	roots []string

	mu    sync.Mutex
	cache map[string]string
}

func New(roots []string) *Finder {
	return &Finder{roots: expandRoots(roots), cache: map[string]string{}}
}

func expandRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	home, _ := os.UserHomeDir()
	for _, r := range roots {
		if strings.HasPrefix(r, "~") && home != "" {
			r = filepath.Join(home, strings.TrimPrefix(r, "~"))
		}
		out = append(out, r)
	}
	return out
}

func (f *Finder) Find(repoName, org string) (string, bool) {
	f.mu.Lock()
	if v, hit := f.cache[repoName]; hit {
		f.mu.Unlock()
		return v, v != ""
	}
	f.mu.Unlock()

	path := f.search(repoName, org)
	f.mu.Lock()
	f.cache[repoName] = path
	f.mu.Unlock()
	return path, path != ""
}

func (f *Finder) search(repoName, org string) string {
	for _, root := range f.roots {
		candidate := filepath.Join(root, repoName)
		if !isGitRepo(candidate) {
			continue
		}
		if remoteMatches(candidate, org, repoName) {
			return candidate
		}
	}
	return ""
}

func isGitRepo(path string) bool {
	st, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return st.IsDir() || st.Mode().IsRegular()
}

func remoteMatches(path, org, repoName string) bool {
	cmd := exec.Command("git", "-C", path, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	needleOrg := "dev.azure.com/" + strings.ToLower(org)
	needleRepo := "/" + strings.ToLower(repoName)
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, needleOrg) && strings.Contains(l, needleRepo) {
			return true
		}
	}
	return false
}
