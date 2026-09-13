package activity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
)

// newProject builds a directory that looks enough like a project for the
// scanner: a .scratchpad to hold the cache, and nothing else.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, project.MetaDir), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func write(t *testing.T, path string, size int, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mod.IsZero() {
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

// freezeClock pins the package clock for the duration of a test.
func freezeClock(t *testing.T, now time.Time) {
	t.Helper()
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = time.Now })
}

func TestScanFindsNewestModTime(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)

	dir := newProject(t)
	old := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-2 * time.Hour)
	write(t, filepath.Join(dir, "README.md"), 10, old)
	write(t, filepath.Join(dir, "src", "main.go"), 20, recent)

	sig := Scan(t.Context(), dir)
	if !sig.ModTime.Equal(recent) {
		t.Errorf("ModTime = %v, want %v", sig.ModTime, recent)
	}
	if got, want := sig.Size, int64(30); got != want {
		t.Errorf("Size = %d, want %d", got, want)
	}
	if sig.Deps != 0 {
		t.Errorf("Deps = %d, want 0", sig.Deps)
	}
}

func TestScanIgnoresDependencyAndMetaDirs(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)

	dir := newProject(t)
	source := now.Add(-20 * 24 * time.Hour)
	churn := now.Add(-time.Minute)
	write(t, filepath.Join(dir, "main.go"), 100, source)
	write(t, filepath.Join(dir, "node_modules", "left-pad", "index.js"), 500, churn)
	write(t, filepath.Join(dir, "target", "debug", "bin"), 400, churn)
	write(t, filepath.Join(dir, ".git", "COMMIT_EDITMSG"), 50, churn)
	write(t, filepath.Join(dir, project.MetaDir, "metadata.json"), 50, churn)

	sig := Scan(t.Context(), dir)

	// An npm install is not work on the project, and neither is Scratchpad
	// writing its own bookkeeping.
	if !sig.ModTime.Equal(source) {
		t.Errorf("ModTime = %v, want %v", sig.ModTime, source)
	}
	// Size still counts every byte the user could reclaim — node_modules and
	// .git included — but not Scratchpad's own bookkeeping.
	if got, want := sig.Size, int64(1050); got != want {
		t.Errorf("Size = %d, want %d", got, want)
	}
	if got, want := sig.Deps, int64(900); got != want {
		t.Errorf("Deps = %d, want %d", got, want)
	}
}

func TestScanIgnoresSymlinks(t *testing.T) {
	freezeClock(t, time.Now())

	dir := newProject(t)
	write(t, filepath.Join(dir, "real.txt"), 100, time.Time{})
	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// A symlink does not occupy its target's bytes, so counting it would
	// overstate what a delete frees.
	if got, want := Scan(t.Context(), dir).Size, int64(100); got != want {
		t.Errorf("Size = %d, want %d", got, want)
	}
}

func TestScanIgnoresFutureTimestamps(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)

	dir := newProject(t)
	past := now.Add(-time.Hour)
	write(t, filepath.Join(dir, "sane.txt"), 10, past)
	write(t, filepath.Join(dir, "skewed.txt"), 10, now.Add(72*time.Hour))

	// A file dated in the future would otherwise pin the project to Active
	// for as long as the clock skew lasts.
	if got := Scan(t.Context(), dir).ModTime; !got.Equal(past) {
		t.Errorf("ModTime = %v, want %v", got, past)
	}
}

// treatAsExpensive makes every project look big enough to be worth caching,
// which beats writing a thousand files into a temp directory.
func treatAsExpensive(t *testing.T) {
	t.Helper()
	cheapFiles = 0
	t.Cleanup(func() { cheapFiles = 1000 })
}

func TestScanAlwaysRewalksCheapProjects(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)

	dir := newProject(t)
	write(t, filepath.Join(dir, "src", "main.go"), 10, now.Add(-30*24*time.Hour))
	first := Scan(t.Context(), dir)

	// This is the case 2.1 exists for: a file edited in an editor, deep enough
	// in the tree that nothing at the top level moved. A handful of files is
	// cheap to walk, so the answer must be right immediately.
	edited := now.Add(-time.Minute)
	write(t, filepath.Join(dir, "src", "main.go"), 999, edited)

	got := Scan(t.Context(), dir)
	if !got.ModTime.Equal(edited) {
		t.Errorf("ModTime = %v, want %v", got.ModTime, edited)
	}
	if got.Size == first.Size {
		t.Errorf("Size = %d, want the rescanned total", got.Size)
	}
}

func TestScanCachesExpensiveTreesUntilTheTopLevelMoves(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)
	treatAsExpensive(t)

	dir := newProject(t)
	write(t, filepath.Join(dir, "src", "main.go"), 10, now.Add(-time.Hour))

	first := Scan(t.Context(), dir)
	if _, err := os.Stat(cachePath(dir)); err != nil {
		t.Fatalf("cache file: %v", err)
	}

	// Rewriting a file in place changes nothing the probe can see: not the
	// root, not src itself. That blind spot is the trade made for never
	// re-walking a 2 GB tree.
	write(t, filepath.Join(dir, "src", "main.go"), 999, now.Add(-time.Minute))
	if got := Scan(t.Context(), dir); got.Size != first.Size {
		t.Errorf("Size = %d after an in-place edit, want the cached %d", got.Size, first.Size)
	}

	// Creating a file does move a directory the probe reads, and must
	// invalidate even though it is a level down.
	write(t, filepath.Join(dir, "src", "extra.go"), 5, time.Time{})
	if got := Scan(t.Context(), dir); got.Size == first.Size {
		t.Errorf("Size = %d, want a rescan after src changed", got.Size)
	}
}

func TestScanCacheSurvivesItsOwnWrite(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)
	treatAsExpensive(t)

	dir := newProject(t)
	write(t, filepath.Join(dir, "src", "main.go"), 10, now.Add(-time.Hour))

	// Writing the cache touches .scratchpad. If the probe counted that, the
	// cache would invalidate itself on every single run.
	Scan(t.Context(), dir)

	cached, ok := load(dir)
	if !ok {
		t.Fatal("no cache was written")
	}
	after, err := probe(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.fresh(after, now) {
		t.Error("the cache went stale the moment it was written")
	}
}

func TestScanCacheExpires(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)
	treatAsExpensive(t)

	dir := newProject(t)
	write(t, filepath.Join(dir, "src", "main.go"), 10, now.Add(-time.Hour))
	first := Scan(t.Context(), dir)

	write(t, filepath.Join(dir, "src", "main.go"), 999, now.Add(-time.Minute))

	freezeClock(t, now.Add(maxAge+time.Minute))
	if got := Scan(t.Context(), dir); got.Size == first.Size {
		t.Errorf("Size = %d, want a rescan once the cache aged out", got.Size)
	}
}

func TestRescanIgnoresTheCache(t *testing.T) {
	now := time.Now()
	freezeClock(t, now)

	dir := newProject(t)
	write(t, filepath.Join(dir, "src", "main.go"), 10, now.Add(-time.Hour))
	first := Scan(t.Context(), dir)

	// The first scan wrote its own cache into .scratchpad, so the totals
	// differ by more than the new file; what matters is that it walked again.
	write(t, filepath.Join(dir, "src", "extra.go"), 999, now.Add(-time.Minute))
	if got := Rescan(t.Context(), dir); got.Size < first.Size+999 {
		t.Errorf("Size = %d, want at least %d", got.Size, first.Size+999)
	}
	treatAsExpensive(t)
	if got := Rescan(t.Context(), dir); got.Size < first.Size+999 {
		t.Errorf("Size = %d with a valid cache in place, want the walked total", got.Size)
	}
}

func TestLatestPrefersTheNewerSignal(t *testing.T) {
	mod := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	commit := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

	if got := (Signals{ModTime: mod, LastCommit: commit}).Latest(); !got.Equal(commit) {
		t.Errorf("Latest = %v, want %v", got, commit)
	}
	if got := (Signals{ModTime: commit, LastCommit: mod}).Latest(); !got.Equal(commit) {
		t.Errorf("Latest = %v, want %v", got, commit)
	}
	if got := (Signals{}).Latest(); !got.IsZero() {
		t.Errorf("Latest = %v, want the zero time", got)
	}
}

func TestScanMissingDirIsZero(t *testing.T) {
	if got := Scan(t.Context(), filepath.Join(t.TempDir(), "nope")); got.Size != 0 || !got.ModTime.IsZero() {
		t.Errorf("Scan = %+v, want zero signals", got)
	}
}

func TestCollectScansEveryDirectory(t *testing.T) {
	freezeClock(t, time.Now())

	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = newProject(t)
		write(t, filepath.Join(dirs[i], "file.txt"), 10*(i+1), time.Time{})
	}

	sigs := Collect(context.Background(), dirs, false)
	if len(sigs) != len(dirs) {
		t.Fatalf("Collect returned %d signals, want %d", len(sigs), len(dirs))
	}
	for i, dir := range dirs {
		if got, want := sigs[dir].Size, int64(10*(i+1)); got != want {
			t.Errorf("%s: Size = %d, want %d", dir, got, want)
		}
	}
}
