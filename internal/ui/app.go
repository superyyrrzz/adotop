// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
	"github.com/renzeyu/adotop/internal/config"
)

type connDataMsg struct {
	data *ado.ConnectionData
	err  error
}

type Model struct {
	cfg    config.Config
	client *ado.Client

	width, height int
	user          string
	loadErr       string
	showHelp      bool
}

func New(cfg config.Config, client *ado.Client) Model {
	return Model{cfg: cfg, client: client, user: "loading…"}
}

func (m Model) Init() tea.Cmd {
	return m.fetchConnectionData()
}

func (m Model) fetchConnectionData() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := m.client.GetConnectionData(ctx)
		return connDataMsg{data: d, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
		case "esc":
			m.showHelp = false
		}
	case connDataMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else if msg.data != nil {
			m.user = msg.data.DisplayName()
			if m.user == "" {
				m.user = "(unknown)"
			}
		}
	}
	return m, nil
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	footerStyle = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	helpBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (m Model) View() string {
	org := m.cfg.Org
	if org == "" {
		org = "(no org configured)"
	}
	project := m.cfg.Project
	if project == "" {
		project = "(no project)"
	}
	header := headerStyle.Render(fmt.Sprintf("adotop  org=%s  project=%s  user=%s", org, project, m.user))
	footer := footerStyle.Render("?: help    q: quit")

	var body string
	if m.loadErr != "" {
		body = errStyle.Render("connectionData failed: " + m.loadErr)
	} else {
		body = "Stage 0 shell. PR/work-item/pipeline screens come in later stages."
	}

	if m.showHelp {
		body = helpBox.Render(strings.Join([]string{
			"Help",
			"",
			"  ?    toggle this help",
			"  q    quit",
			"  esc  close help",
		}, "\n"))
	}

	return strings.Join([]string{header, "", body, "", footer}, "\n")
}

// Run starts the Bubble Tea program with the alt screen.
func Run(cfg config.Config, client *ado.Client) error {
	p := tea.NewProgram(New(cfg, client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
