package tui

import "charm.land/bubbles/v2/key"

// keyMap is the dashboard's bindings.
//
// Everything that changes a project is bound to a capital letter and nothing
// else. It keeps the whole lowercase alphabet free for navigation, and it means
// no single relaxed keystroke can move or bin a directory. Quitting, searching
// and help are harmless, so they answer to either case.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Top    key.Binding
	Bottom key.Binding
	Open   key.Binding
	New    key.Binding
	Keep   key.Binding
	Trash  key.Binding
	Rename key.Binding
	Search key.Binding
	Clear  key.Binding
	Help   key.Binding
	Quit   key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("↓/j", "down")),
		Top:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "first")),
		Bottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "last")),
		Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		New:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new")),
		Keep:   key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "keep")),
		Trash:  key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "trash")),
		Rename: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename")),
		Search: key.NewBinding(key.WithKeys("S", "s", "/"), key.WithHelp("s", "search")),
		Clear:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear search")),
		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:   key.NewBinding(key.WithKeys("q", "Q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.New, k.Keep, k.Trash, k.Search, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Open, k.New, k.Keep},
		{k.Trash, k.Rename, k.Search},
		{k.Clear, k.Help, k.Quit},
	}
}
