package gitx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/InvalidJoker/scratchpad/internal/gitx"
)

// repo makes a git repository with a deterministic identity, so commits work
// on machines with no global git config.
func repo(t *testing.T) string {
	t.Helper()
	if !gitx.Available() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if err := gitx.Init(context.Background(), dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestReadNonRepoIsNotAnError(t *testing.T) {
	status, err := gitx.Read(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if status != nil {
		t.Errorf("status = %+v, want nil for a directory with no repo", status)
	}
	// A nil status must behave as "nothing at risk" rather than panic.
	if !status.Clean() {
		t.Error("nil status reported as dirty")
	}
	if got := status.Summary(); got != "" {
		t.Errorf("Summary = %q, want empty", got)
	}
}

func TestReadEmptyRepo(t *testing.T) {
	status, err := gitx.Read(context.Background(), repo(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if status.Commits != 0 {
		t.Errorf("Commits = %d, want 0", status.Commits)
	}
	if !status.Clean() {
		t.Error("an empty repo has nothing at risk, want Clean")
	}
}

func TestReadDetectsUncommittedWork(t *testing.T) {
	dir := repo(t)
	if err := os.WriteFile(filepath.Join(dir, "draft.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := gitx.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Dirty) != 1 {
		t.Fatalf("Dirty = %v, want one entry", status.Dirty)
	}
	if status.Clean() {
		t.Error("uncommitted work reported as clean")
	}
	if got := status.Summary(); got != "1 uncommitted file" {
		t.Errorf("Summary = %q, want %q", got, "1 uncommitted file")
	}
}

func TestReadIgnoresScratchpadMetadata(t *testing.T) {
	dir := repo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".scratchpad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".scratchpad", "metadata.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := gitx.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Dirty) != 0 {
		t.Errorf("Dirty = %v, want our own metadata ignored", status.Dirty)
	}
	if !status.Clean() {
		t.Error("Scratchpad's own bookkeeping counted as the user's work at risk")
	}
}

func TestLocalOnlyCommitsCountAsAtRisk(t *testing.T) {
	dir := repo(t)
	run(t, dir, "commit", "--allow-empty", "-m", "first")
	run(t, dir, "commit", "--allow-empty", "-m", "second")

	status, err := gitx.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Commits != 2 {
		t.Errorf("Commits = %d, want 2", status.Commits)
	}
	if status.Clean() {
		t.Error("commits that exist on no remote are still at risk, want not Clean")
	}
	if got := status.Summary(); got != "2 commits and no remote" {
		t.Errorf("Summary = %q", got)
	}
	if status.LastCommit.IsZero() {
		t.Error("LastCommit not read")
	}
}

func TestPushedWorkIsClean(t *testing.T) {
	origin := t.TempDir()
	run(t, origin, "init", "--bare", "--quiet")

	dir := repo(t)
	run(t, dir, "commit", "--allow-empty", "-m", "first")
	run(t, dir, "remote", "add", "origin", origin)
	run(t, dir, "push", "--quiet", "--set-upstream", "origin", "HEAD")

	status, err := gitx.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.HasRemote || !status.HasUpstream {
		t.Fatalf("remote=%v upstream=%v, want both", status.HasRemote, status.HasUpstream)
	}
	if status.Unpushed != 0 {
		t.Errorf("Unpushed = %d, want 0", status.Unpushed)
	}
	if !status.Clean() {
		t.Errorf("fully pushed work reported as at risk: %q", status.Summary())
	}
}

func TestUnpushedCommitsAreCounted(t *testing.T) {
	origin := t.TempDir()
	run(t, origin, "init", "--bare", "--quiet")

	dir := repo(t)
	run(t, dir, "commit", "--allow-empty", "-m", "first")
	run(t, dir, "remote", "add", "origin", origin)
	run(t, dir, "push", "--quiet", "--set-upstream", "origin", "HEAD")
	run(t, dir, "commit", "--allow-empty", "-m", "local only")

	status, err := gitx.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Unpushed != 1 {
		t.Errorf("Unpushed = %d, want 1", status.Unpushed)
	}
	if got := status.Summary(); got != "1 unpushed commit" {
		t.Errorf("Summary = %q, want %q", got, "1 unpushed commit")
	}
}
