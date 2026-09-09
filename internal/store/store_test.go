package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

// testStore returns a store rooted in a temp directory with a fixed clock.
func testStore(t *testing.T, now time.Time) *store.Store {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		ScratchDir:        filepath.Join(root, "scratch"),
		ProjectsDir:       filepath.Join(root, "projects"),
		ArchiveDir:        filepath.Join(root, "archive"),
		TrashDir:          filepath.Join(root, "scratch", ".trash"),
		DefaultExpiration: config.Duration(30 * 24 * time.Hour),
		StaleAfter:        config.Duration(14 * 24 * time.Hour),
	}
	s := store.New(cfg)
	s.SetClock(func() time.Time { return now })
	if err := s.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	return s
}

func TestCreateWritesMetadata(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	p, err := s.Create(store.CreateOptions{
		Name:        "weather-app",
		Description: "Testing a new weather UI",
		Tags:        []string{"experiment"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got, want := p.Dir(), s.PathFor(store.Scratch, "weather-app"); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if p.State != project.StateActive {
		t.Errorf("State = %q, want %q", p.State, project.StateActive)
	}
	if want := now.Add(30 * 24 * time.Hour); !p.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", p.ExpiresAt, want)
	}
	if _, err := os.Stat(p.MetaPath()); err != nil {
		t.Errorf("metadata file: %v", err)
	}

	// Metadata must survive a round trip through disk.
	got, err := s.Get("weather-app")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != p.Description || !got.Created.Equal(p.Created) {
		t.Errorf("round trip mismatch: %+v vs %+v", got, p)
	}
}

func TestCreatePermanentLandsInProjectsDir(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	p, err := s.Create(store.CreateOptions{Name: "portfolio", Permanent: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got, want := p.Dir(), s.PathFor(store.Kept, "portfolio"); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if p.State != project.StateKept {
		t.Errorf("State = %q, want %q", p.State, project.StateKept)
	}
	if !p.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero: kept projects do not expire", p.ExpiresAt)
	}
}

func TestCreateNeverExpiresWithZeroTTL(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "forever", TTL: 0, TTLSet: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero", p.ExpiresAt)
	}
}

func TestCreateRejectsDuplicate(t *testing.T) {
	s := testStore(t, time.Now())
	if _, err := s.Create(store.CreateOptions{Name: "dup"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := s.Create(store.CreateOptions{Name: "dup"})
	if !errors.Is(err, store.ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
}

func TestCreateRejectsPathTraversal(t *testing.T) {
	s := testStore(t, time.Now())
	for _, name := range []string{"../escape", "a/b", ".hidden", "", "."} {
		if _, err := s.Create(store.CreateOptions{Name: name}); err == nil {
			t.Errorf("Create(%q) succeeded, want error", name)
		}
	}
}

func TestCreateLeavesNoLitterOnFailure(t *testing.T) {
	s := testStore(t, time.Now())
	// Make the scratch dir read-only so metadata cannot be written.
	if err := os.Chmod(s.Dir(store.Scratch), 0o500); err != nil {
		t.Skipf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(s.Dir(store.Scratch), 0o755) })

	if _, err := s.Create(store.CreateOptions{Name: "doomed"}); err == nil {
		t.Skip("filesystem allowed the write; nothing to assert")
	}
	if _, err := os.Stat(s.PathFor(store.Scratch, "doomed")); !os.IsNotExist(err) {
		t.Errorf("failed create left a directory behind")
	}
}

func TestGetReportsNotFound(t *testing.T) {
	s := testStore(t, time.Now())
	_, err := s.Get("nope")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListSkipsUnmanagedDirsAndSortsByActivity(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	// Three projects, each created a day apart.
	for i, name := range []string{"oldest", "middle", "newest"} {
		s.SetClock(func() time.Time { return now.Add(time.Duration(i) * 24 * time.Hour) })
		if _, err := s.Create(store.CreateOptions{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}
	// A plain directory that Scratchpad does not manage.
	if err := os.MkdirAll(filepath.Join(s.Dir(store.Scratch), "random-download"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(store.Scratch)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"newest", "middle", "oldest"}
	if len(got) != len(want) {
		t.Fatalf("got %d projects, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestListMissingDirIsEmptyNotError(t *testing.T) {
	s := testStore(t, time.Now())
	got, err := s.List(store.Archive) // never created
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d projects, want 0", len(got))
	}
}

func TestLoadDirTrustsDirectoryName(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "before"})
	if err != nil {
		t.Fatal(err)
	}
	renamed := s.PathFor(store.Scratch, "after")
	if err := os.Rename(p.Dir(), renamed); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadDir(renamed)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if got.Name != "after" {
		t.Errorf("Name = %q, want %q: the directory name is authoritative", got.Name, "after")
	}
}
