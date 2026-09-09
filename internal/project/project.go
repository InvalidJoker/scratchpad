// Package project defines the on-disk model for a scratch project: its
// metadata file and the lifecycle rules derived from it.
package project

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MetaDir is the per-project directory holding Scratchpad's bookkeeping.
const MetaDir = ".scratchpad"

const MetaFile = "metadata.json"

// State is a project's persisted lifecycle state. Staleness is deliberately
// not a state: it is derived from activity, see Project.Status.
type State string

const (
	// StateActive is a live temporary project.
	StateActive State = "active"
	// StateKept is a project promoted out of the scratch directory.
	StateKept State = "kept"
	// StateArchived is a project compressed into the archive directory.
	StateArchived State = "archived"
	// StateTrashed is a project sitting in the recovery area.
	StateTrashed State = "trashed"
)

// Status is what the UI shows: the persisted state, refined by activity.
type Status string

const (
	StatusActive   Status = "active"
	StatusStale    Status = "stale"
	StatusExpired  Status = "expired"
	StatusKept     Status = "kept"
	StatusArchived Status = "archived"
	StatusTrashed  Status = "trashed"
)

func (s Status) Icon() string {
	switch s {
	case StatusActive:
		return "🟢"
	case StatusStale:
		return "🟡"
	case StatusExpired:
		return "🟠"
	case StatusKept:
		return "🔵"
	case StatusArchived:
		return "📦"
	case StatusTrashed:
		return "🔴"
	}
	return "⚪"
}

func (s Status) String() string { return string(s) }

// Project is the metadata Scratchpad stores for a single scratch project. It
// is serialized to <project>/.scratchpad/metadata.json.
type Project struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	State       State     `json:"state"`
	Created     time.Time `json:"created"`
	LastOpened  time.Time `json:"last_opened"`
	// ExpiresAt is when the project becomes a cleanup candidate. A zero value
	// means it never expires.
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	// OpenCount is how many times the project has been opened, a cheap signal
	// for "you keep coming back to this one".
	OpenCount int `json:"open_count"`
	// Note is free-form context written by `sp note`.
	Note      string    `json:"note,omitempty"`
	TrashedAt time.Time `json:"trashed_at,omitzero"`
	// OriginalPath is where the project lived before being trashed or kept,
	// so `sp restore` knows where to put it back.
	OriginalPath string `json:"original_path,omitempty"`

	// dir is the project's directory on disk. It is set by the store on load
	// and never serialized, so moving a project on disk cannot desync it.
	dir string
}

func (p *Project) Dir() string { return p.dir }

// SetDir records where the project lives. The store owns this.
func (p *Project) SetDir(dir string) { p.dir = dir }

func (p *Project) MetaPath() string { return filepath.Join(p.dir, MetaDir, MetaFile) }

// Status derives the display status from state and activity. staleAfter is the
// inactivity window after which an active project reads as stale; zero
// disables staleness.
func (p *Project) Status(now time.Time, staleAfter time.Duration) Status {
	switch p.State {
	case StateKept:
		return StatusKept
	case StateArchived:
		return StatusArchived
	case StateTrashed:
		return StatusTrashed
	}
	if p.IsExpired(now) {
		return StatusExpired
	}
	if staleAfter > 0 && now.Sub(p.LastActivity()) >= staleAfter {
		return StatusStale
	}
	return StatusActive
}

// LastActivity is the most recent point at which the project showed signs of
// life. Git and filesystem signals are folded in by the caller via
// RecordActivity; this is the metadata-only floor.
func (p *Project) LastActivity() time.Time {
	t := p.LastOpened
	if p.Created.After(t) {
		t = p.Created
	}
	return t
}

// RecordActivity moves the activity floor forward if ts is more recent. It is
// how filesystem and git signals feed into staleness.
func (p *Project) RecordActivity(ts time.Time) {
	if ts.After(p.LastOpened) {
		p.LastOpened = ts
	}
}

func (p *Project) IsExpired(now time.Time) bool {
	return !p.ExpiresAt.IsZero() && now.After(p.ExpiresAt)
}

// IsTemporary reports whether the project is still on the scratch lifecycle.
func (p *Project) IsTemporary() bool { return p.State == StateActive }

func (p *Project) Age(now time.Time) time.Duration { return now.Sub(p.Created) }

// Idle is how long the project has gone without activity.
func (p *Project) Idle(now time.Time) time.Duration { return now.Sub(p.LastActivity()) }

// HasTag reports whether the project carries tag, case-insensitively.
func (p *Project) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// AddTag adds a tag if it is not already present, and reports whether it did.
func (p *Project) AddTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" || p.HasTag(tag) {
		return false
	}
	p.Tags = append(p.Tags, tag)
	return true
}

func (p *Project) Touch(now time.Time) {
	p.LastOpened = now
	p.OpenCount++
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateName rejects names that would be unsafe or confusing as a directory
// under the scratch root.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("project name cannot be empty")
	case len(name) > 100:
		return fmt.Errorf("project name is too long (max 100 characters)")
	case !nameRe.MatchString(name):
		return fmt.Errorf("invalid project name %q: use letters, digits, dots, dashes and underscores, starting with a letter or digit", name)
	case name == MetaDir:
		return fmt.Errorf("%q is reserved", name)
	}
	return nil
}
