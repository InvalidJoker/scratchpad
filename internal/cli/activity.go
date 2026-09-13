package cli

import (
	"context"

	"github.com/InvalidJoker/scratchpad/internal/activity"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

// scanned runs a query with filesystem and git activity folded in first, and
// returns the matching projects alongside the signals gathered on the way.
//
// The order matters. Everything in the searched locations is loaded and
// scanned before the filter runs, because the filter itself asks questions —
// is this stale, has it been idle for 30 days — that only the scan can answer
// honestly. Filtering first would be faster and wrong.
//
// force re-walks every tree instead of trusting the cache, for the callers
// whose whole output is the numbers.
func (a *App) scanned(ctx context.Context, f store.Filter, force bool) ([]*project.Project, map[string]activity.Signals, error) {
	all, err := a.store.Collect(f.Locations)
	if err != nil {
		return nil, nil, err
	}
	signals := a.refreshActivity(ctx, all, force)
	return a.store.Match(all, f), signals, nil
}

// refreshActivity scans the given projects and moves each one's activity floor
// up to whatever the filesystem and git report.
func (a *App) refreshActivity(ctx context.Context, ps []*project.Project, force bool) map[string]activity.Signals {
	dirs := make([]string, 0, len(ps))
	for _, p := range ps {
		dirs = append(dirs, p.Dir())
	}

	signals := activity.Collect(ctx, dirs, force)
	for _, p := range ps {
		// Persist only when the scan actually found something newer. Most runs
		// find nothing, and a listing should not rewrite every metadata file
		// it reads.
		if p.RecordActivity(signals[p.Dir()].Latest()) {
			// A project that cannot be written to still lists; the cost of the
			// failure is that its activity is recomputed next time.
			_ = a.store.Save(p)
		}
	}
	return signals
}
