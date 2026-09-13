package cli

import (
	"context"

	"github.com/InvalidJoker/scratchpad/internal/tui"
)

// runDashboard opens the TUI, or prints the listing when there is no terminal
// to draw on.
//
// The fallback is not a degraded mode so much as the honest answer to what
// `sp` was asked for: `sp | less` and `sp > out.txt` want the projects, and a
// full-screen UI has nothing to say to either.
func (a *App) runDashboard(ctx context.Context) error {
	if !a.hasTerminal() {
		return a.runList(ctx, listOptions{sortKey: string(defaultSort)})
	}
	if err := a.store.EnsureDirs(); err != nil {
		return err
	}
	return tui.Run(ctx, a.store, a.cfg)
}
