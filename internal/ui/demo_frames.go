package ui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

const (
	demoFrameWidth  = 118
	demoFrameHeight = 34
)

// WriteDemoFrames emits a small set of real TUI views separated by form-feed
// characters. It is intentionally used only by recording tooling; the public
// interactive path is `adotop demo`.
func WriteDemoFrames(w io.Writer, cfg config.Config, prs []ado.PRSummary) error {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	frames := demoFrames(cfg, prs)
	for i, f := range frames {
		if i > 0 {
			fmt.Fprint(w, "\f")
		}
		fmt.Fprint(w, f)
	}
	return nil
}

func demoFrames(cfg config.Config, prs []ado.PRSummary) []string {
	m := New(cfg, nil)
	m.cache = nil
	m.user = "Alice Anderson"
	m.myID = "user-alice"
	m.detail = m.detail.SetMyID(m.myID)
	m, _ = demoApply(m, tea.WindowSizeMsg{Width: demoFrameWidth, Height: demoFrameHeight})
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})

	frames := []string{m.View()}

	summary := prs[0]
	detail := demoDetail(summary)
	files := demoFiles()
	threads := demoUIThreads()
	statuses := demoStatuses()
	m.detail = m.detail.SetSummary(summary)
	m.detail, _ = m.detail.Update(detailLoadedMsg{detail: detail})
	m.detail, _ = m.detail.Update(filesLoadedMsg{files: files})
	m.detail, _ = m.detail.Update(statusesLoadedMsg{statuses: statuses})
	m.threads = threads
	m.detail = m.detail.SetPRThreads(threads, false)
	m.detail = m.detail.SetMyVoteIsStale(true)
	m.myVoteIsStale = true
	m.screen = screenDetail
	m.detailFocus = focusFiles
	m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
	m.preview, _ = m.preview.Update(diffLoadedMsg{content: demoDiff(), target: diffTargetPreview})
	m.previewKey = "demo"
	frames = append(frames, m.View())

	m.detailFocus = focusDiff
	frames = append(frames, m.View())

	m.pendingAction = pendingAction{kind: "abandon", prompt: "Abandon PR #1145087? (y/N)"}
	frames = append(frames, m.View())
	m.pendingAction = pendingAction{}
	m.detail = m.detail.SetMyVote(10, m.myID, m.user)
	m.footerOK = "PR #1145087 voted approve"
	frames = append(frames, m.View())

	m.showHelp = true
	frames = append(frames, m.View())

	return frames
}

func demoApply(m Model, msg tea.Msg) (Model, tea.Cmd) {
	mm, cmd := m.Update(msg)
	if out, ok := mm.(Model); ok {
		return out, cmd
	}
	return m, cmd
}

func demoDetail(s ado.PRSummary) *ado.PRDetail {
	return &ado.PRDetail{
		PRSummary:     s,
		DescriptionMD: "Prepare the April service release. Includes retry hardening, clearer telemetry, and one small session-state fix.",
		WorkItemRefs:  []ado.WorkItemRef{{ID: "7421", URL: "https://dev.azure.com/fabrikam/_apis/wit/workItems/7421"}},
		SourceSha:     "a1111111111111111111111111111111111111111",
		TargetSha:     "b2222222222222222222222222222222222222222",
	}
}

func demoFiles() []ado.FileChange {
	return []ado.FileChange{
		{Path: "/src/api/session.go", ChangeType: "edit"},
		{Path: "/src/api/handlers.go", ChangeType: "edit"},
		{Path: "/src/api/token_store.go", ChangeType: "add"},
		{Path: "/README.md", ChangeType: "edit"},
	}
}

func demoStatuses() []ado.StatusCheck {
	return []ado.StatusCheck{
		{Context: "ci/lint", State: "succeeded"},
		{Context: "ci/unit-tests", State: "succeeded"},
		{Context: "policy/required-reviewers", State: "succeeded"},
	}
}

func demoUIThreads() []ado.Thread {
	return []ado.Thread{
		{
			ID:        401,
			Status:    "active",
			FilePath:  "/src/api/session.go",
			RightLine: 47,
			Comments: []ado.Comment{
				{ID: 1, Author: "Carol Chen", Content: "Should we also refresh the expiry timestamp here?", Type: "text"},
				{ID: 2, Author: "Bob Brown", Content: "Good catch. I added a guard so callers do not keep a stale session.", Type: "text"},
			},
		},
		{
			ID:       450,
			Status:   "active",
			Comments: []ado.Comment{{ID: 1, Author: "Required Reviewers", Content: "Ownership check passed for src/api.", Type: "system"}},
		},
	}
}

func demoDiff() []byte {
	return []byte(strings.TrimPrefix(`
diff --git a/src/api/session.go b/src/api/session.go
index 4f2a8bc..9c31d42 100644
--- a/src/api/session.go
+++ b/src/api/session.go
@@ -1,13 +1,15 @@
 package api

 import "time"

 type Session struct {
 	token string
 	expiresAt time.Time
+	refreshedAt time.Time
 }

 func (s *Session) Refresh(newToken string) error {
 	if s.expiresAt.Before(time.Now()) {
 		return ErrExpired
 	}
 	s.token = newToken
+	s.refreshedAt = time.Now()
 	return nil
 }
`, "\n"))
}
