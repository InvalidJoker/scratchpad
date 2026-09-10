package gitx

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Status is the safety picture Scratchpad shows before destroying a project.
type Status struct {
	Branch string
	// Commits is the number of commits reachable from HEAD.
	Commits int
	// Dirty lists modified, staged and untracked paths.
	Dirty []string
	// Unpushed counts commits ahead of the upstream branch.
	Unpushed int
	// HasUpstream is false for a branch that was never pushed, which makes
	// every one of its commits at risk.
	HasUpstream bool
	HasRemote   bool
	LastCommit  time.Time
}

// Clean reports whether there is no work at risk: nothing uncommitted and
// nothing unpushed.
func (s *Status) Clean() bool {
	if s == nil {
		return true
	}
	if len(s.Dirty) > 0 {
		return false
	}
	if s.Commits == 0 {
		return true
	}
	// Commits that exist nowhere but this directory are at risk exactly like
	// uncommitted files are.
	if !s.HasRemote || !s.HasUpstream {
		return false
	}
	return s.Unpushed == 0
}

// Summary describes the risk in one line, or "" when nothing is at risk.
func (s *Status) Summary() string {
	if s == nil || s.Clean() {
		return ""
	}
	var parts []string
	if n := len(s.Dirty); n > 0 {
		parts = append(parts, plural(n, "uncommitted file"))
	}
	switch {
	case s.Commits == 0:
	case !s.HasRemote:
		parts = append(parts, plural(s.Commits, "commit")+" and no remote")
	case !s.HasUpstream:
		parts = append(parts, plural(s.Commits, "commit")+" on an unpushed branch")
	case s.Unpushed > 0:
		parts = append(parts, plural(s.Unpushed, "unpushed commit"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// Read collects the git status of dir. It returns nil for a directory that is
// not a repository at all, which callers treat as "no git signal", not as an
// error.
func Read(ctx context.Context, dir string) (*Status, error) {
	if !IsRepo(dir) || !Available() {
		return nil, nil
	}

	s := &Status{}
	if out, err := output(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		s.Branch = out
	}
	// An empty repository has no HEAD, so a failure here means zero commits
	// rather than a broken repo.
	if out, err := output(ctx, dir, "rev-list", "--count", "HEAD"); err == nil {
		s.Commits, _ = strconv.Atoi(out)
	}
	if out, err := output(ctx, dir, "status", "--porcelain"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				s.Dirty = append(s.Dirty, line)
			}
		}
	}
	if out, err := output(ctx, dir, "remote"); err == nil {
		s.HasRemote = out != ""
	}
	if out, err := output(ctx, dir, "rev-list", "--count", "@{upstream}..HEAD"); err == nil {
		s.HasUpstream = true
		s.Unpushed, _ = strconv.Atoi(out)
	}
	if out, err := output(ctx, dir, "log", "-1", "--format=%cI"); err == nil && out != "" {
		if t, err := time.Parse(time.RFC3339, out); err == nil {
			s.LastCommit = t
		}
	}
	return s, nil
}

// LastCommit returns the time of the most recent commit, or the zero time when
// there is none.
func LastCommit(ctx context.Context, dir string) time.Time {
	if !IsRepo(dir) || !Available() {
		return time.Time{}
	}
	out, err := output(ctx, dir, "log", "-1", "--format=%cI")
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return time.Time{}
	}
	return t
}

func output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &Error{Args: args, Msg: strings.TrimSpace(stderr.String()), Err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}
