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

// Stamped at build time via -ldflags; goreleaser fills both in for releases.
var (
	version = "dev"
	commit  = ""
)

func main() {
	// Ctrl-C should cancel in-flight work (git, archiving) rather than leave
	// half-written state behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// fang rewrites root.Version from its own options, so the stamped values
	// have to be handed to it explicitly or `sp --version` reports nothing.
	err := fang.Execute(ctx, cli.NewRootCommand(version),
		fang.WithErrorHandler(cli.ErrorHandler),
		fang.WithVersion(version),
		fang.WithCommit(commit),
	)
	if err != nil {
		os.Exit(1)
	}
}
