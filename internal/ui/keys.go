package ui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up, Down         key.Binding
	NextTab, PrevTab key.Binding
	Open, Back       key.Binding
	Refresh          key.Binding
	Browser          key.Binding
	Filter           key.Binding
	Help, Quit       key.Binding
	PgUp, PgDn       key.Binding
	GotoTop, GotoEnd key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Up:      key.NewBinding(key.WithKeys("k", "up")),
		Down:    key.NewBinding(key.WithKeys("j", "down")),
		NextTab: key.NewBinding(key.WithKeys("tab", "l")),
		PrevTab: key.NewBinding(key.WithKeys("shift+tab", "h")),
		Open:    key.NewBinding(key.WithKeys("enter")),
		Back:    key.NewBinding(key.WithKeys("esc")),
		Refresh: key.NewBinding(key.WithKeys("r")),
		Browser: key.NewBinding(key.WithKeys("o")),
		Filter:  key.NewBinding(key.WithKeys("/")),
		Help:    key.NewBinding(key.WithKeys("?")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c")),
		PgUp:    key.NewBinding(key.WithKeys("pgup")),
		PgDn:    key.NewBinding(key.WithKeys("pgdown")),
		GotoTop: key.NewBinding(key.WithKeys("g")),
		GotoEnd: key.NewBinding(key.WithKeys("G")),
	}
}
