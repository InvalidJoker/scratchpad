// Command sp is Scratchpad: a lifecycle manager for experimental projects.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/InvalidJoker/scratchpad/internal/cli"
	"github.com/charmbracelet/fang"
)

// version is stamped at build time via -ldflags.
var version = "dev"

func main() {
	// Ctrl-C should cancel in-flight work (git, archiving) rather than leave
	// half-written state behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := fang.Execute(ctx, cli.NewRootCommand(version),
		fang.WithErrorHandler(cli.ErrorHandler),
	)
	if err != nil {
		os.Exit(1)
	}
}
