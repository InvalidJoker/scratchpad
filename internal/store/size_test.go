package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/InvalidJoker/scratchpad/internal/store"
)

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "nested", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, size := range map[string]int{
		"a.txt":                  100,
		"nested/b.txt":           250,
		"nested/deep/c.txt":      1000,
		"nested/deep/node_stuff": 500,
	} {
		if err := os.WriteFile(filepath.Join(dir, path), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Everything counts: the point of the number is how much space comes back.
	if got, want := store.DirSize(dir), int64(1850); got != want {
		t.Errorf("DirSize = %d, want %d", got, want)
	}
}

func TestDirSizeIgnoresSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// A symlink does not occupy its target's bytes, so counting it would
	// double-count and overstate what a delete frees.
	if got, want := store.DirSize(dir), int64(100); got != want {
		t.Errorf("DirSize = %d, want %d", got, want)
	}
}

func TestDirSizeMissingDirIsZero(t *testing.T) {
	if got := store.DirSize(filepath.Join(t.TempDir(), "nope")); got != 0 {
		t.Errorf("DirSize = %d, want 0", got)
	}
}
