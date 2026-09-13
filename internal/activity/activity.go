// Package activity derives the signals Scratchpad's own metadata cannot
// provide: when a project's files last changed, when it was last committed to,
// and how much disk it occupies.
//
// Every one of those answers costs a walk of the project tree, which the
// listing path cannot afford. A scan is therefore cached inside the project,
// next to its metadata, and only retaken when the guard in Scan says it has
// to be.
package activity

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
)

// cacheName is the scan cache, stored beside the project's metadata.
const cacheName = "activity.json"

// maxAge bounds how long a cached scan is trusted. See Scan for why a time
// limit is needed on top of the modification-time probe.
const maxAge = 15 * time.Minute

// cheapFiles is the tree size below which the cache is skipped entirely: a
// walk of a few hundred files costs well under a millisecond, so there is no
// reason to answer from a cache that might be wrong. It is a variable only so
// tests can lower it.
var cheapFiles = 1000

// fanOut caps concurrent scans. Disk is the bottleneck, so a hundred parallel
// tree walks are slower than a handful, not faster.
const fanOut = 8

// clock is the package clock, swapped in tests.
var clock = time.Now

// Signals is everything a scan found out about a project's contents.
type Signals struct {
	// ModTime is the newest modification time in the tree, ignoring the
	// directories that churn without meaning.
	ModTime time.Time `json:"mod_time,omitzero"`
	// LastCommit is the time of the project's most recent commit.
	LastCommit time.Time `json:"last_commit,omitzero"`
	// Size is every byte the project occupies, dependency directories
	// included: the number answers "how much space does deleting this free",
	// and that space is real however uninteresting the files are.
	Size int64 `json:"size"`
	// Deps is the part of Size sitting in directories a build can recreate.
	Deps int64 `json:"deps"`
	// Files counts the regular files walked. It is what the scan cost, and
	// therefore what decides whether the cache below is worth consulting.
	Files int `json:"files"`

	// ScannedAt and Probe are the cache guard, not results. See Scan.
	ScannedAt time.Time `json:"scanned_at,omitzero"`
	Probe     time.Time `json:"probe,omitzero"`
}

// Latest is the most recent sign of life the scan found, or the zero time when
// it found none.
func (s Signals) Latest() time.Time {
	if s.LastCommit.After(s.ModTime) {
		return s.LastCommit
	}
	return s.ModTime
}

// depDirs hold files a build tool can recreate. They are excluded from the
// activity signal — installing dependencies is not working on a project — and
// counted separately so their bytes can be reported as reclaimable.
var depDirs = map[string]bool{
	"node_modules": true,
	"target":       true,
	"dist":         true,
	"venv":         true,
	".venv":        true,
}

// noiseDirs churn for reasons that have nothing to do with the user working on
// the project, so neither the activity signal nor the freshness probe may read
// their timestamps. Scratchpad's own directory is the load-bearing one: the
// cache below is written into it, and a probe that watched it would invalidate
// itself on every run.
var noiseDirs = map[string]bool{
	".git":          true,
	project.MetaDir: true,
}

// Scan returns the signals for a project directory, reusing the cached scan
// only where walking again would actually cost something.
//
// Most scratch projects are a few dozen source files, and walking those is
// cheaper than being wrong about them, so they are rescanned every time. The
// cache is for the trees that would hurt: a project carrying a node_modules,
// which the performance rule says must never be walked on the listing path.
// For those, the probe below catches anything moving at the top level the
// moment it happens, and maxAge is the backstop for an edit buried deeper.
//
// A directory that cannot be read yields zero signals rather than an error: one
// broken project must not take a listing down with it.
func Scan(ctx context.Context, dir string) Signals {
	p, err := probe(dir)
	if err != nil {
		return Signals{}
	}
	if cached, ok := load(dir); ok && cached.fresh(p, clock()) {
		return cached
	}
	return scan(ctx, dir, p)
}

// Rescan walks the tree whatever the cache says. It is for the places where
// the number on screen is the whole point, such as the space `sp clean`
// promises to reclaim.
func Rescan(ctx context.Context, dir string) Signals {
	p, err := probe(dir)
	if err != nil {
		return Signals{}
	}
	return scan(ctx, dir, p)
}

// Collect scans several projects at once, keyed by directory.
func Collect(ctx context.Context, dirs []string, force bool) map[string]Signals {
	out := make(map[string]Signals, len(dirs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, fanOut)

	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sig := Scan(ctx, dir)
			if force {
				sig = Rescan(ctx, dir)
			}
			mu.Lock()
			out[dir] = sig
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func (s Signals) fresh(p, now time.Time) bool {
	if s.ScannedAt.IsZero() || s.Files <= cheapFiles || !s.Probe.Equal(p) {
		return false
	}
	// A negative age means the cache was written by a clock ahead of ours, or
	// copied in from another machine. Neither is worth trusting.
	age := now.Sub(s.ScannedAt)
	return age >= 0 && age < maxAge
}

func scan(ctx context.Context, dir string, p time.Time) Signals {
	now := clock()
	sc := scanner{now: now}
	// The root's own mtime is noted here rather than in the walk, which only
	// ever sees a directory through its parent's entry. Deleting a top-level
	// file leaves no other trace.
	if info, err := os.Stat(dir); err == nil {
		sc.note(info.ModTime())
	}
	sc.walk(dir, false, false)

	sig := sc.sig
	// A commit dated in the future is a skewed clock, not activity.
	if commit := gitx.LastCommit(ctx, dir); !commit.After(now) {
		sig.LastCommit = commit
	}
	sig.ScannedAt = now
	sig.Probe = p
	save(dir, sig)
	return sig
}

type scanner struct {
	now time.Time
	sig Signals
}

// walk descends one directory. inDeps and inNoise are inherited rather than
// recomputed, because what a file counts for depends on the whole path above
// it, not just on its parent.
func (sc *scanner) walk(dir string, inDeps, inNoise bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// One unreadable subtree should not lose the whole scan; an
		// approximate answer is more useful than none.
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			// A symlink owns none of its target's bytes and none of its
			// history, and following one risks walking in a circle.
			continue
		case e.IsDir():
			if e.Name() == project.MetaDir {
				// Scratchpad's own bookkeeping is not part of the project. The
				// scan cache lives in here, so counting it would make a
				// project's reported size grow every time it was looked at.
				continue
			}
			deps := inDeps || depDirs[e.Name()]
			noise := inNoise || deps || noiseDirs[e.Name()]
			if !noise {
				// A directory's mtime moves when a file is created or deleted
				// inside it, which is as real a signal as an edit.
				sc.note(info.ModTime())
			}
			sc.walk(filepath.Join(dir, e.Name()), deps, noise)
		case info.Mode().IsRegular():
			sc.sig.Files++
			sc.sig.Size += info.Size()
			if inDeps {
				sc.sig.Deps += info.Size()
			}
			if !inNoise {
				sc.note(info.ModTime())
			}
		}
	}
}

func (sc *scanner) note(ts time.Time) {
	// A timestamp in the future is a bad clock or an unpacked archive, not
	// activity; recording it would pin the project to Active forever.
	if ts.After(sc.sig.ModTime) && !ts.After(sc.now) {
		sc.sig.ModTime = ts
	}
}

// probe is the cheap freshness check: the newest modification time among the
// project root and everything directly inside it. One ReadDir, whatever the
// tree below costs.
//
// Scratchpad's own directory and .git are left out. Both change for reasons
// that are not the user's work — writing the cache below is one of them — and
// a probe that invalidates itself would defeat the whole cache.
func probe(dir string) (time.Time, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return time.Time{}, err
	}
	if !info.IsDir() {
		return time.Time{}, fs.ErrInvalid
	}

	newest := info.ModTime()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest, nil
	}
	for _, e := range entries {
		if noiseDirs[e.Name()] {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return newest, nil
}

func cachePath(dir string) string {
	return filepath.Join(dir, project.MetaDir, cacheName)
}

func load(dir string) (Signals, bool) {
	data, err := os.ReadFile(cachePath(dir))
	if err != nil {
		return Signals{}, false
	}
	var s Signals
	if err := json.Unmarshal(data, &s); err != nil {
		// A truncated cache is a miss, not a failure: the next scan replaces
		// it.
		return Signals{}, false
	}
	return s, true
}

// save writes the cache atomically, like every other write Scratchpad makes.
// Every failure here is swallowed: a cache that could not be written costs
// another walk next time and nothing else, and a read-only project should
// still list.
func save(dir string, s Signals) {
	metaDir := filepath.Join(dir, project.MetaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}

	tmp, err := os.CreateTemp(metaDir, ".activity-*.json")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	os.Chmod(tmp.Name(), 0o644)
	_ = os.Rename(tmp.Name(), cachePath(dir))
}
