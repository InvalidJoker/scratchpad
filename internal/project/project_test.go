package project_test

import (
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
)

const day = 24 * time.Hour

func TestStatusDerivesFromActivity(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	staleAfter := 14 * day

	tests := []struct {
		name string
		p    project.Project
		want project.Status
	}{
		{
			name: "recently opened",
			p:    project.Project{State: project.StateActive, Created: now.Add(-30 * day), LastOpened: now.Add(-2 * time.Hour), ExpiresAt: now.Add(day)},
			want: project.StatusActive,
		},
		{
			name: "untouched past the stale window",
			p:    project.Project{State: project.StateActive, Created: now.Add(-40 * day), LastOpened: now.Add(-18 * day), ExpiresAt: now.Add(day)},
			want: project.StatusStale,
		},
		{
			name: "expiry beats staleness",
			p:    project.Project{State: project.StateActive, Created: now.Add(-40 * day), LastOpened: now.Add(-18 * day), ExpiresAt: now.Add(-day)},
			want: project.StatusExpired,
		},
		{
			name: "kept never goes stale",
			p:    project.Project{State: project.StateKept, Created: now.Add(-400 * day), LastOpened: now.Add(-300 * day)},
			want: project.StatusKept,
		},
		{
			name: "trashed stays trashed",
			p:    project.Project{State: project.StateTrashed, Created: now.Add(-40 * day), LastOpened: now.Add(-40 * day)},
			want: project.StatusTrashed,
		},
		{
			name: "no expiry set never expires",
			p:    project.Project{State: project.StateActive, Created: now.Add(-400 * day), LastOpened: now.Add(-time.Hour)},
			want: project.StatusActive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Status(now, staleAfter); got != tc.want {
				t.Errorf("Status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusWithStalenessDisabled(t *testing.T) {
	now := time.Now()
	p := project.Project{State: project.StateActive, Created: now.Add(-400 * day), LastOpened: now.Add(-400 * day)}
	if got := p.Status(now, 0); got != project.StatusActive {
		t.Errorf("Status = %q, want %q", got, project.StatusActive)
	}
}

func TestRecordActivityOnlyMovesForward(t *testing.T) {
	now := time.Now()
	p := project.Project{LastOpened: now}

	p.RecordActivity(now.Add(-time.Hour))
	if !p.LastOpened.Equal(now) {
		t.Errorf("an older signal moved LastOpened backwards to %v", p.LastOpened)
	}

	later := now.Add(time.Hour)
	p.RecordActivity(later)
	if !p.LastOpened.Equal(later) {
		t.Errorf("LastOpened = %v, want %v", p.LastOpened, later)
	}
}

func TestLastActivityFallsBackToCreation(t *testing.T) {
	created := time.Now().Add(-3 * day)
	p := project.Project{Created: created}
	if got := p.LastActivity(); !got.Equal(created) {
		t.Errorf("LastActivity = %v, want %v", got, created)
	}
}

func TestTags(t *testing.T) {
	p := project.Project{}
	if !p.AddTag("Experiment") {
		t.Fatal("AddTag returned false for a new tag")
	}
	if p.AddTag("experiment") {
		t.Error("AddTag added a case-insensitive duplicate")
	}
	if p.AddTag("  ") {
		t.Error("AddTag added a blank tag")
	}
	if !p.HasTag("EXPERIMENT") {
		t.Error("HasTag should match case-insensitively")
	}
}

func TestTouchCountsOpens(t *testing.T) {
	now := time.Now()
	p := project.Project{}
	p.Touch(now)
	p.Touch(now.Add(time.Hour))
	if p.OpenCount != 2 {
		t.Errorf("OpenCount = %d, want 2", p.OpenCount)
	}
	if !p.LastOpened.Equal(now.Add(time.Hour)) {
		t.Errorf("LastOpened = %v, want the most recent touch", p.LastOpened)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"weather-app", "a", "v2.0", "my_project", "3d-test"}
	for _, n := range valid {
		if err := project.ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []string{"", ".", "..", "../etc", "a/b", ".hidden", "-leading", "has space", ".scratchpad"}
	for _, n := range invalid {
		if err := project.ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", n)
		}
	}
}
