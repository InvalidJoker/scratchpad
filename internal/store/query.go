package store

import (
	"sort"
	"strings"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
)

type SortKey string

const (
	// SortActivity orders by most recently active first, which is the order
	// you actually want when scanning a list of projects.
	SortActivity SortKey = "used"
	SortName     SortKey = "name"
	SortAge      SortKey = "age"
)

// Filter selects and orders projects. A zero Filter returns everything in the
// scratch directory, most recently active first.
type Filter struct {
	// Locations to search. Empty means the scratch directory alone.
	Locations []Location
	// Statuses to include. Empty means every status.
	Statuses []project.Status
	// Tag matches projects carrying it, case-insensitively.
	Tag string
	// Query is a substring matched against name, description, note and tags.
	Query string
	// IdleFor keeps only projects untouched for at least this long.
	IdleFor time.Duration
	Sort    SortKey
	Reverse bool
}

// StatusOf derives a project's display status using the configured staleness
// window.
func (s *Store) StatusOf(p *project.Project) project.Status {
	return p.Status(s.now(), s.cfg.StaleAfter.Duration())
}

// Query returns the projects matching f, sorted.
func (s *Store) Query(f Filter) ([]*project.Project, error) {
	all, err := s.Collect(f.Locations)
	if err != nil {
		return nil, err
	}
	return s.Match(all, f), nil
}

// Collect loads every managed project in the given locations, unfiltered. Empty
// locations means the scratch directory alone.
func (s *Store) Collect(locs []Location) ([]*project.Project, error) {
	if len(locs) == 0 {
		locs = []Location{Scratch}
	}

	var out []*project.Project
	seen := map[string]bool{}
	for _, loc := range locs {
		found, err := s.List(loc)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			// Two locations can resolve to the same directory in a hand-edited
			// config; a project should still appear only once.
			if seen[p.Dir()] {
				continue
			}
			seen[p.Dir()] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// Match filters and sorts projects already in hand.
//
// It is separate from Query so that a caller which has folded filesystem and
// git activity into the projects first can filter on the refreshed values.
// Selecting on staleness with metadata the scan has already contradicted would
// be exactly the lie this layer exists to stop telling.
func (s *Store) Match(ps []*project.Project, f Filter) []*project.Project {
	now := s.now()
	staleAfter := s.cfg.StaleAfter.Duration()

	out := make([]*project.Project, 0, len(ps))
	for _, p := range ps {
		if matches(p, f, now, staleAfter) {
			out = append(out, p)
		}
	}

	sortProjects(out, f.Sort, now)
	if f.Reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

func matches(p *project.Project, f Filter, now time.Time, staleAfter time.Duration) bool {
	if len(f.Statuses) > 0 {
		status := p.Status(now, staleAfter)
		if !containsStatus(f.Statuses, status) {
			return false
		}
	}
	if f.Tag != "" && !p.HasTag(f.Tag) {
		return false
	}
	if f.IdleFor > 0 && p.Idle(now) < f.IdleFor {
		return false
	}
	if f.Query != "" && !matchesQuery(p, f.Query) {
		return false
	}
	return true
}

func matchesQuery(p *project.Project, query string) bool {
	q := strings.ToLower(query)
	for _, field := range []string{p.Name, p.Description, p.Note} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	for _, t := range p.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

func containsStatus(list []project.Status, want project.Status) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func sortProjects(ps []*project.Project, key SortKey, now time.Time) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		switch key {
		case SortName:
			return a.Name < b.Name
		case SortAge:
			return a.Created.Before(b.Created)
		default:
			if a.LastActivity().Equal(b.LastActivity()) {
				return a.Name < b.Name
			}
			return a.LastActivity().After(b.LastActivity())
		}
	})
}

// Counts tallies projects by status, for the summary line under a listing.
func (s *Store) Counts(ps []*project.Project) map[project.Status]int {
	counts := make(map[project.Status]int, len(ps))
	for _, p := range ps {
		counts[s.StatusOf(p)]++
	}
	return counts
}
