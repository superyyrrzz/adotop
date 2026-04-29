package ui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up, Down              key.Binding
	NextTab, PrevTab      key.Binding
	Open, Back            key.Binding
	Refresh               key.Binding
	Browser               key.Binding
	Filter                key.Binding
	Help, Quit            key.Binding
	QuitForce             key.Binding
	PgUp, PgDn            key.Binding
	GotoTop, GotoEnd      key.Binding
	NextFile, PrevFile    key.Binding
	Approve, Abandon      key.Binding
	VoteMenu              key.Binding
	ConfirmYes, ConfirmNo key.Binding
	JumpToID              key.Binding
	ShowResolved          key.Binding
	WrapDiff              key.Binding
	CtxMore, CtxLess      key.Binding
	NextThread, PrevThread key.Binding
	ToggleResolve          key.Binding
	ComposeThread          key.Binding
	ReplyThread            key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Up:         key.NewBinding(key.WithKeys("k", "up")),
		Down:       key.NewBinding(key.WithKeys("j", "down")),
		NextTab:    key.NewBinding(key.WithKeys("tab", "l")),
		PrevTab:    key.NewBinding(key.WithKeys("shift+tab", "h")),
		Open:       key.NewBinding(key.WithKeys("enter")),
		Back:       key.NewBinding(key.WithKeys("esc")),
		Refresh:    key.NewBinding(key.WithKeys("r")),
		Browser:    key.NewBinding(key.WithKeys("o")),
		Filter:     key.NewBinding(key.WithKeys("/")),
		Help:       key.NewBinding(key.WithKeys("?")),
		Quit:       key.NewBinding(key.WithKeys("q")),
		QuitForce:  key.NewBinding(key.WithKeys("ctrl+c")),
		PgUp:       key.NewBinding(key.WithKeys("pgup")),
		PgDn:       key.NewBinding(key.WithKeys("pgdown")),
		GotoTop:    key.NewBinding(key.WithKeys("g")),
		GotoEnd:    key.NewBinding(key.WithKeys("G")),
		NextFile:   key.NewBinding(key.WithKeys("n")),
		PrevFile:   key.NewBinding(key.WithKeys("N")),
		Approve:    key.NewBinding(key.WithKeys("a")),
		Abandon:    key.NewBinding(key.WithKeys("X")),
		VoteMenu:   key.NewBinding(key.WithKeys("v")),
		ConfirmYes: key.NewBinding(key.WithKeys("y", "Y")),
		ConfirmNo:  key.NewBinding(key.WithKeys("esc")),
		JumpToID:   key.NewBinding(key.WithKeys("#")),
		ShowResolved: key.NewBinding(key.WithKeys("R")),
		WrapDiff:     key.NewBinding(key.WithKeys("w")),
		CtxMore:      key.NewBinding(key.WithKeys("+", "=")),
		CtxLess:      key.NewBinding(key.WithKeys("-", "_")),
		NextThread:   key.NewBinding(key.WithKeys("]")),
		PrevThread:   key.NewBinding(key.WithKeys("[")),
		ToggleResolve: key.NewBinding(key.WithKeys("x")),
		ComposeThread: key.NewBinding(key.WithKeys("c")),
		ReplyThread:   key.NewBinding(key.WithKeys("C")),
	}
}
