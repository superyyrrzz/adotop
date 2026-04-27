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
}

type diffTarget int

const (
	diffTargetFull diffTarget = iota
	diffTargetPreview
)

type DiffModel struct {
	keys     KeyMap
	file     string
	renderer string
	vp       viewport.Model
	loadErr  string
	loaded   bool
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
	m.loaded = false
	m.loadErr = ""
	m.vp.SetContent("loading…")
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
			m.vp.SetContent(string(Colorize(msg.content)))
			m.vp.GotoTop()
		}
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.GotoTop):
			m.vp.GotoTop()
		case keyMatches(msg, m.keys.GotoEnd):
			m.vp.GotoBottom()
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m DiffModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", Header.Render(m.file), Faint.Render("["+m.renderer+"]"))
	b.WriteString(m.vp.View())
	if m.loadErr != "" {
		b.WriteString("\n" + ErrLine.Render(m.loadErr))
	}
	return b.String()
}
