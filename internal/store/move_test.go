package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

// writeFile puts a file inside a project so moves can be checked for content
// survival, not just directory existence.
func writeFile(t *testing.T, p *project.Project, name, content string) {
	t.Helper()
	path := filepath.Join(p.Dir(), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestKeepMovesAndStopsExpiry(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	p, err := s.Create(store.CreateOptions{Name: "awesome-app"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, "src/main.go", "package main")
	origin := p.Dir()

	if err := s.Keep(p, ""); err != nil {
		t.Fatalf("Keep: %v", err)
	}

	if want := s.PathFor(store.Kept, "awesome-app"); p.Dir() != want {
		t.Errorf("Dir = %q, want %q", p.Dir(), want)
	}
	if p.State != project.StateKept {
		t.Errorf("State = %q, want kept", p.State)
	}
	if !p.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero: a kept project must stop expiring", p.ExpiresAt)
	}
	if _, err := os.Stat(origin); !os.IsNotExist(err) {
		t.Error("the original directory still exists after a keep")
	}
	if got := readFile(t, p.Dir(), "src/main.go"); got != "package main" {
		t.Errorf("file content = %q, want it to survive the move", got)
	}

	// The move must be visible to a fresh read, not just in memory.
	reloaded, err := s.Get("awesome-app", store.Kept)
	if err != nil {
		t.Fatalf("Get after keep: %v", err)
	}
	if reloaded.State != project.StateKept {
		t.Errorf("persisted State = %q, want kept", reloaded.State)
	}
}

func TestKeepToCustomDirectory(t *testing.T) {
	s := testStore(t, time.Now())
	dest := t.TempDir()

	p, err := s.Create(store.CreateOptions{Name: "portfolio"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Keep(p, dest); err != nil {
		t.Fatalf("Keep: %v", err)
	}
	if want := filepath.Join(dest, "portfolio"); p.Dir() != want {
		t.Errorf("Dir = %q, want %q", p.Dir(), want)
	}
}

func TestKeepRefusesToClobber(t *testing.T) {
	s := testStore(t, time.Now())

	p, err := s.Create(store.CreateOptions{Name: "clash"})
	if err != nil {
		t.Fatal(err)
	}
	blocker := s.PathFor(store.Kept, "clash")
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}

	err = s.Keep(p, "")
	if !errors.Is(err, store.ErrOccupied) {
		t.Fatalf("err = %v, want ErrOccupied", err)
	}
	var occupied *store.OccupiedError
	if !errors.As(err, &occupied) || occupied.Path != blocker {
		t.Errorf("error does not carry the blocking path: %v", err)
	}
	// The project must not have moved.
	if p.Dir() != s.PathFor(store.Scratch, "clash") {
		t.Errorf("Dir = %q, want the project left where it was", p.Dir())
	}
}

func TestKeepIsIdempotentlyRejected(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "already", Permanent: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Keep(p, ""); err == nil {
		t.Error("keeping an already-kept project succeeded, want an error")
	}
}

func TestTrashAndRestoreRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	p, err := s.Create(store.CreateOptions{Name: "old-website"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, "index.html", "<h1>hi</h1>")
	origin := p.Dir()

	if err := s.Trash(p); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if p.State != project.StateTrashed {
		t.Errorf("State = %q, want trashed", p.State)
	}
	if p.TrashedAt.IsZero() {
		t.Error("TrashedAt not stamped")
	}
	if p.OriginalPath != origin {
		t.Errorf("OriginalPath = %q, want %q", p.OriginalPath, origin)
	}
	if p.Dir() != s.PathFor(store.Trash, "old-website") {
		t.Errorf("Dir = %q, want it in the trash", p.Dir())
	}

	if err := s.Restore(p); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if p.Dir() != origin {
		t.Errorf("Dir = %q, want restored to %q", p.Dir(), origin)
	}
	if p.State != project.StateActive {
		t.Errorf("State = %q, want active", p.State)
	}
	if !p.TrashedAt.IsZero() || p.OriginalPath != "" {
		t.Error("restore left trash bookkeeping behind")
	}
	if p.ExpiresAt.IsZero() {
		t.Error("restored project has no expiry, want a fresh window")
	}
	if got := readFile(t, p.Dir(), "index.html"); got != "<h1>hi</h1>" {
		t.Errorf("content = %q, want it to survive the round trip", got)
	}
}

func TestTrashDisambiguatesCollisions(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	for i := 0; i < 2; i++ {
		p, err := s.Create(store.CreateOptions{Name: "dup"})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		if err := s.Trash(p); err != nil {
			t.Fatalf("Trash %d: %v", i, err)
		}
	}

	trashed, err := s.List(store.Trash)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 2 {
		t.Fatalf("got %d trashed projects, want 2: a repeat delete must not overwrite the first", len(trashed))
	}
}

func TestRestoreFallsBackToScratchWhenOriginIsTaken(t *testing.T) {
	s := testStore(t, time.Now())

	p, err := s.Create(store.CreateOptions{Name: "revived"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Keep(p, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(p); err != nil {
		t.Fatal(err)
	}
	// Something else now occupies the projects directory slot.
	if err := os.MkdirAll(s.PathFor(store.Kept, "revived"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(p); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if want := s.PathFor(store.Scratch, "revived"); p.Dir() != want {
		t.Errorf("Dir = %q, want the scratch fallback %q", p.Dir(), want)
	}
}

func TestRestoreRefusesWhenBothPathsAreTaken(t *testing.T) {
	s := testStore(t, time.Now())

	p, err := s.Create(store.CreateOptions{Name: "blocked"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateOptions{Name: "blocked"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(p); !errors.Is(err, store.ErrOccupied) {
		t.Fatalf("err = %v, want ErrOccupied", err)
	}
}

func TestRestoreRejectsUntrashedProject(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "live"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(p); err == nil {
		t.Error("restoring a live project succeeded, want an error")
	}
}

func TestDestroyRemovesEverything(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "doomed"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, "a/b/c.txt", "bye")

	if err := s.Destroy(p); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := os.Stat(p.Dir()); !os.IsNotExist(err) {
		t.Error("directory survived Destroy")
	}
}

func TestDestroyRefusesUnmanagedDirectory(t *testing.T) {
	s := testStore(t, time.Now())

	// A project whose metadata directory is gone must not be rm -rf'd: the
	// path may not be what we think it is.
	p, err := s.Create(store.CreateOptions{Name: "corrupt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(p.Dir(), project.MetaDir)); err != nil {
		t.Fatal(err)
	}

	if err := s.Destroy(p); !errors.Is(err, store.ErrNotManaged) {
		t.Fatalf("err = %v, want ErrNotManaged", err)
	}
	if _, err := os.Stat(p.Dir()); err != nil {
		t.Error("Destroy removed a directory it should have refused")
	}
}

func TestMovePreservesSymlinksAndModTimes(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "linky"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, "real.txt", "content")
	if err := os.Symlink("real.txt", filepath.Join(p.Dir(), "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stamp := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(p.Dir(), "real.txt"), stamp, stamp); err != nil {
		t.Fatal(err)
	}

	// Exercise the copy path directly: on a single-filesystem test machine
	// os.Rename always succeeds, so the fallback would never be covered.
	dst := filepath.Join(t.TempDir(), "copied")
	if err := store.CopyTreeForTest(p.Dir(), dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	if got := readFile(t, dst, "real.txt"); got != "content" {
		t.Errorf("content = %q, want %q", got, "content")
	}

	info, err := os.Lstat(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was copied as a regular file, want it kept as a link")
	}
	target, err := os.Readlink(filepath.Join(dst, "link.txt"))
	if err != nil || target != "real.txt" {
		t.Errorf("link target = %q (err %v), want %q", target, err, "real.txt")
	}

	// Modification times are the raw material for staleness detection, so a
	// cross-device move must not reset them to now.
	copied, err := os.Stat(filepath.Join(dst, "real.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !copied.ModTime().Equal(stamp) {
		t.Errorf("mtime = %v, want %v preserved through the copy", copied.ModTime(), stamp)
	}
}
