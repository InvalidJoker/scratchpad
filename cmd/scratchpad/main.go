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

// Stamped at build time via -ldflags
var (
	version = "dev"
	commit  = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := fang.Execute(ctx, cli.NewRootCommand(version),
		fang.WithErrorHandler(cli.ErrorHandler),
		fang.WithVersion(version),
		fang.WithCommit(commit),
	)
	if err != nil {
		os.Exit(1)
	}
}
