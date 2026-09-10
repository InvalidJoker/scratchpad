// Package scaffold fills a freshly created project directory with the
// starting files a scratch project deserves.
package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
)

type Options struct {
	Git    bool
	Readme bool
}

// Apply writes the starter files into p's directory. Existing files are never
// overwritten, so scaffolding an already-populated directory is safe.
func Apply(ctx context.Context, p *project.Project, opts Options) error {
	if opts.Readme {
		if err := writeIfAbsent(filepath.Join(p.Dir(), "README.md"), readme(p)); err != nil {
			return err
		}
	}
	if opts.Git {
		if !gitx.Available() {
			return fmt.Errorf("git is not installed")
		}
		if err := gitx.Init(ctx, p.Dir()); err != nil {
			return err
		}
		// Commit the scaffold so the project starts from a clean baseline.
		// This needs a git identity, and plenty of machines do not have one
		// configured; that is not worth failing or warning about, so a failure
		// just leaves the files staged for the user.
		_ = gitx.CommitAll(ctx, p.Dir(), "Initial scratch")
	}
	return nil
}

func readme(p *project.Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", p.Description)
	}
	fmt.Fprintf(&b, "Scratch project, created %s.\n", p.Created.Format("2006-01-02"))
	if !p.ExpiresAt.IsZero() {
		fmt.Fprintf(&b, "Expires %s unless kept with `sp keep %s`.\n", p.ExpiresAt.Format("2006-01-02"), p.Name)
	}
	return b.String()
}

func writeIfAbsent(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(content)
	return err
}
