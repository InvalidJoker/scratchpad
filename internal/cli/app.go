// Package cli assembles Scratchpad's command tree.
package cli

import (
	"io"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

// App is the state shared by every command: the loaded config, the store
// built on top of it, and where output goes.
type App struct {
	cfg   *config.Config
	store *store.Store
	out   io.Writer
}

func (a *App) Config() *config.Config { return a.cfg }

func (a *App) Store() *store.Store { return a.store }

func (a *App) printf(format string, args ...any) {
	if len(args) == 0 {
		io.WriteString(a.out, format)
		return
	}
	io.WriteString(a.out, sprintf(format, args...))
}

func (a *App) println(lines ...string) {
	for _, l := range lines {
		io.WriteString(a.out, l+"\n")
	}
}
