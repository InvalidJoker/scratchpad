// Package cli assembles Scratchpad's command tree.
package cli

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

// App is the state shared by every command: the loaded config, the store
// built on top of it, and where output goes.
type App struct {
	cfg   *config.Config
	store *store.Store
	out   io.Writer
	in    io.Reader
}

// rebuildStore points the store at the current config, after setup has
// replaced it.
func (a *App) rebuildStore() { a.store = store.New(a.cfg) }

// interactive reports whether there is a human to prompt. Destructive commands
// refuse to guess when there is not.
func (a *App) interactive() bool {
	f, ok := a.in.(term.File)
	return ok && term.IsTerminal(f.Fd())
}

// stdio returns the streams to hand to a child process such as an editor.
func (a *App) stdio() (io.Reader, io.Writer, io.Writer) {
	return os.Stdin, os.Stdout, os.Stderr
}

func (a *App) Config() *config.Config { return a.cfg }

func (a *App) Store() *store.Store { return a.store }

func (a *App) println(lines ...string) {
	for _, l := range lines {
		// A failed write to the terminal has nowhere useful to be reported.
		_, _ = io.WriteString(a.out, l+"\n")
	}
}
