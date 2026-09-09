// Package gitx wraps the small slice of git Scratchpad needs: detecting
// repositories and reading the safety signals shown before a delete.
package gitx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func IsRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// Init creates a repository in dir. It is a no-op if one already exists.
func Init(ctx context.Context, dir string) error {
	if IsRepo(dir) {
		return nil
	}
	return run(ctx, dir, "init", "--quiet")
}

// run executes a git command in dir, folding stderr into the returned error.
func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return &Error{Args: args, Msg: msg, Err: err}
		}
		return &Error{Args: args, Err: err}
	}
	return nil
}

// Error carries the git invocation that failed and whatever it printed.
type Error struct {
	Args []string
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	s := "git " + strings.Join(e.Args, " ")
	if e.Msg != "" {
		return s + ": " + e.Msg
	}
	return s + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }
