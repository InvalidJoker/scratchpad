package store_test

import (
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

const day = 24 * time.Hour

// seed creates a project and backdates its activity by idle.
func seed(t *testing.T, s *store.Store, now time.Time, name string, idle time.Duration, tags ...string) *project.Project {
	t.Helper()
	p, err := s.Create(store.CreateOptions{Name: name, Tags: tags})
	if err != nil {
		t.Fatalf("Create(%s): %v", name, err)
	}
	p.Created = now.Add(-idle - 3*day)
	p.LastOpened = now.Add(-idle)
	p.ExpiresAt = p.Created.Add(30 * day)
	if err := s.Save(p); err != nil {
		t.Fatalf("Save(%s): %v", name, err)
	}
	return p
}

func names(ps []*project.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// queryFixture builds a store holding one project per interesting status.
func queryFixture(t *testing.T) (*store.Store, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := testStore(t, now)

	seed(t, s, now, "weather-app", 2*time.Hour, "frontend")
	seed(t, s, now, "threejs-test", 18*day, "experiment")
	seed(t, s, now, "old-website", 61*day)

	kept, err := s.Create(store.CreateOptions{Name: "portfolio", Permanent: true})
	if err != nil {
		t.Fatal(err)
	}
	kept.Description = "Trying a waveform UI"
	if err := s.Save(kept); err != nil {
		t.Fatal(err)
	}
	return s, now
}

func TestQueryDefaultsToScratchByActivity(t *testing.T) {
	s, _ := queryFixture(t)

	got, err := s.Query(store.Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	want := []string{"weather-app", "threejs-test", "old-website"}
	if !equal(names(got), want) {
		t.Errorf("got %v, want %v (kept projects must not leak into scratch listings)", names(got), want)
	}
}

func TestQueryByStatus(t *testing.T) {
	s, _ := queryFixture(t)

	tests := []struct {
		name   string
		filter store.Filter
		want   []string
	}{
		{
			name:   "active only",
			filter: store.Filter{Statuses: []project.Status{project.StatusActive}},
			want:   []string{"weather-app"},
		},
		{
			name:   "stale only",
			filter: store.Filter{Statuses: []project.Status{project.StatusStale}},
			want:   []string{"threejs-test"},
		},
		{
			name:   "expired only",
			filter: store.Filter{Statuses: []project.Status{project.StatusExpired}},
			want:   []string{"old-website"},
		},
		{
			name: "kept lives in another location",
			filter: store.Filter{
				Locations: []store.Location{store.Kept},
				Statuses:  []project.Status{project.StatusKept},
			},
			want: []string{"portfolio"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Query(tc.filter)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if !equal(names(got), tc.want) {
				t.Errorf("got %v, want %v", names(got), tc.want)
			}
		})
	}
}

func TestQueryIdleFor(t *testing.T) {
	s, _ := queryFixture(t)

	got, err := s.Query(store.Filter{IdleFor: 30 * day})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"old-website"}; !equal(names(got), want) {
		t.Errorf("got %v, want %v", names(got), want)
	}
}

func TestQueryTagAndText(t *testing.T) {
	s, _ := queryFixture(t)

	got, err := s.Query(store.Filter{Tag: "EXPERIMENT"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"threejs-test"}; !equal(names(got), want) {
		t.Errorf("tag: got %v, want %v", names(got), want)
	}

	// Text search covers the description, not just the name.
	got, err = s.Query(store.Filter{Locations: []store.Location{store.Kept}, Query: "waveform"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"portfolio"}; !equal(names(got), want) {
		t.Errorf("query: got %v, want %v", names(got), want)
	}
}

func TestQuerySortAndReverse(t *testing.T) {
	s, _ := queryFixture(t)

	got, err := s.Query(store.Filter{Sort: store.SortName})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"old-website", "threejs-test", "weather-app"}; !equal(names(got), want) {
		t.Errorf("by name: got %v, want %v", names(got), want)
	}

	got, err = s.Query(store.Filter{Sort: store.SortAge})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"old-website", "threejs-test", "weather-app"}; !equal(names(got), want) {
		t.Errorf("by age: got %v, want %v", names(got), want)
	}

	got, err = s.Query(store.Filter{Sort: store.SortName, Reverse: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if want := []string{"weather-app", "threejs-test", "old-website"}; !equal(names(got), want) {
		t.Errorf("reversed: got %v, want %v", names(got), want)
	}
}

func TestQueryDeduplicatesOverlappingLocations(t *testing.T) {
	s, _ := queryFixture(t)

	got, err := s.Query(store.Filter{Locations: []store.Location{store.Scratch, store.Scratch}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d projects, want 3: the same location twice must not duplicate", len(got))
	}
}

func TestCounts(t *testing.T) {
	s, _ := queryFixture(t)

	all, err := s.Query(store.Filter{Locations: []store.Location{store.Scratch, store.Kept}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	counts := s.Counts(all)
	want := map[project.Status]int{
		project.StatusActive:  1,
		project.StatusStale:   1,
		project.StatusExpired: 1,
		project.StatusKept:    1,
	}
	for status, n := range want {
		if counts[status] != n {
			t.Errorf("counts[%s] = %d, want %d", status, counts[status], n)
		}
	}
}
