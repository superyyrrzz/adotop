package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type diffLoadedMsg struct {
	content   []byte
	err       error
	target    diffTarget
	requestID int
	// rendered, if non-empty, is the precolorized form of `content`.
	// The cache-hit path in app.go fills this so we don't pay for a
	// second Colorize pass on every j/k. When empty we fall back to
	// running Colorize ourselves.
	rendered string
}

type diffTarget int

const (
	diffTargetFull diffTarget = iota
	diffTargetPreview
)

type DiffModel struct {
	keys      KeyMap
	file      string
	renderer  string
	vp        viewport.Model
	loadErr   string
	loaded    bool
	reloading bool
}

func NewDiff(keys KeyMap) DiffModel {
	vp := viewport.New(80, 20)
	return DiffModel{keys: keys, vp: vp}
}

func (m DiffModel) SetSize(width, height int) DiffModel {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	m.vp.Width = width
	m.vp.Height = height
	return m
}

func (m DiffModel) SetHeader(file, renderer string) DiffModel {
	m.file = file
	m.renderer = renderer
	m.loadErr = ""
	if !m.loaded {
		m.vp.SetContent("loading…")
	}
	m.reloading = m.loaded
	return m
}

func (m DiffModel) Update(msg tea.Msg) (DiffModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 4
	case diffLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			m.vp.SetContent("error: " + m.loadErr)
		} else {
			m.loaded = true
			m.reloading = false
			if msg.rendered != "" {
				m.vp.SetContent(msg.rendered)
			} else {
				m.vp.SetContent(string(Colorize(msg.content)))
			}
			m.vp.GotoTop()
		}
	case tea.KeyMsg:
		// g/G are intercepted at the app level (see updateDetailScreen)
		// so they can dispatch to the file list or the diff viewport
		// based on focus. Nothing to do here.
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m DiffModel) View() string {
	var b strings.Builder
	suffix := "[" + m.renderer + "]"
	if m.reloading {
		suffix += " (reloading…)"
	}
	fmt.Fprintf(&b, "%s  %s\n", Header.Render(m.file), Faint.Render(suffix))
	b.WriteString(m.vp.View())
	if m.loadErr != "" {
		b.WriteString("\n" + ErrLine.Render(m.loadErr))
	}
	return b.String()
}
